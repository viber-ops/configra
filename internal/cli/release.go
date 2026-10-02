package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	configra "github.com/viber-ops/configra-go"
)

func releaseCommand(settings *options) *cobra.Command {
	group := &cobra.Command{Use: "release", Short: "Prepare, validate, activate and roll back a complete Config+File set"}
	var input, operation string
	prepare := &cobra.Command{Use: "prepare CONFIG RELEASE", Args: resourceArgs(2), RunE: func(command *cobra.Command, args []string) error {
		data, err := readInput(command, input, 128<<10)
		if err != nil {
			return err
		}
		defer clear(data)
		var spec configra.ReleaseSpec
		if err := safeJSON(data, &spec); err != nil {
			return err
		}
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			result, err := client.PrepareRelease(ctx, selected.Environment, args[0], args[1], configra.ReleasePrepare{OperationID: operation, ReleaseSpec: spec})
			if err != nil {
				return err
			}
			return settings.emit(command, result)
		})
	}}
	fileFlag(prepare, &input)
	operationFlag(prepare, &operation)
	group.AddCommand(prepare)
	group.AddCommand(&cobra.Command{Use: "state CONFIG", Args: resourceArgs(1), RunE: func(command *cobra.Command, args []string) error {
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			result, err := client.ReadReleaseState(ctx, selected.Environment, args[0])
			if err != nil {
				return err
			}
			return settings.emit(command, result)
		})
	}})
	var directory, key string
	get := &cobra.Command{Use: "get CONFIG", Args: resourceArgs(1), RunE: func(command *cobra.Command, args []string) error {
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			bundle, err := client.ReadRelease(ctx, selected.Environment, args[0], key, "")
			if err != nil {
				return err
			}
			if err := exportRelease(directory, bundle); err != nil {
				return err
			}
			return settings.emit(command, map[string]any{"release_key": bundle.Manifest.ReleaseKey, "generation": bundle.Generation, "digest": bundle.Digest, "written": true})
		})
	}}
	get.Flags().StringVar(&key, "release", "", "Prepared key; omitted follows the active release")
	get.Flags().StringVar(&directory, "output-dir", "", "New private directory; all members are written before success")
	_ = get.MarkFlagRequired("output-dir")
	group.AddCommand(get)
	for _, name := range []string{"activate", "rollback"} {
		var operation, validator string
		var validatorArgs []string
		var expected uint64
		activate := &cobra.Command{Use: name + " CONFIG RELEASE", Short: "Validate the exact candidate locally, then compare-and-swap the active release", Args: resourceArgs(2), RunE: func(command *cobra.Command, args []string) error {
			if !filepath.IsAbs(validator) {
				return fail(2, "validator_requires_absolute_executable_path")
			}
			return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
				bundle, err := client.ReadRelease(ctx, selected.Environment, args[0], args[1], "")
				if err != nil {
					return err
				}
				if err := validateRelease(ctx, bundle, validator, validatorArgs); err != nil {
					return err
				}
				result, err := client.ActivateRelease(ctx, selected.Environment, args[0], args[1], configra.ReleaseActivate{OperationID: operation, ExpectedGeneration: expected})
				if err != nil {
					return err
				}
				return settings.emit(command, result)
			})
		}}
		operationFlag(activate, &operation)
		activate.Flags().Uint64Var(&expected, "expected-generation", 0, "Current stream generation; explicit 0 for first activation")
		activate.Flags().StringVar(&validator, "validator", "", "Local validation executable; no shell expansion; its output is discarded")
		activate.Flags().StringArrayVar(&validatorArgs, "validator-arg", nil, "One argument per flag; candidate directory is in CONFIGRA_RELEASE_DIR")
		_ = activate.MarkFlagRequired("expected-generation")
		_ = activate.MarkFlagRequired("validator")
		group.AddCommand(activate)
	}
	return group
}

func releaseContents(bundle configra.ReleaseBundle) map[string][]byte {
	files := map[string][]byte{bundle.Manifest.ConfigPath: []byte(bundle.Config.Content)}
	for _, file := range bundle.Files {
		files[file.Path] = file.Bytes
	}
	return files
}

// OpenRoot confines every relative write, including on filesystems with links.
// Consumers switch to the completed directory; this does not overwrite a live tree.
func exportRelease(directory string, bundle configra.ReleaseBundle) (resultErr error) {
	if directory == "" || os.Mkdir(directory, 0700) != nil {
		return fail(7, "release_output_requires_new_directory")
	}
	defer func() {
		if resultErr != nil {
			_ = os.RemoveAll(directory)
		}
	}()
	root, err := os.OpenRoot(directory)
	if err != nil {
		return fail(7, "cannot_open_release_output")
	}
	defer root.Close()
	for path, value := range releaseContents(bundle) {
		if !configra.ValidReleasePath(path) {
			return fail(5, "invalid_release_path")
		}
		if root.MkdirAll(filepath.Dir(path), 0700) != nil {
			return fail(7, "cannot_create_release_directory")
		}
		file, err := root.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fail(7, "cannot_create_release_file")
		}
		_, writeErr := file.Write(value)
		syncErr := file.Sync()
		closeErr := file.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			return fail(7, "cannot_write_release_file")
		}
	}
	return nil
}

func validateRelease(ctx context.Context, bundle configra.ReleaseBundle, executable string, args []string) error {
	temporary, err := os.MkdirTemp("", "configra-release-")
	if err != nil {
		return fail(7, "cannot_prepare_release_validation")
	}
	defer os.RemoveAll(temporary)
	directory := filepath.Join(temporary, "candidate")
	if err := exportRelease(directory, bundle); err != nil {
		return err
	}
	discard, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return fail(7, "cannot_prepare_release_validation")
	}
	defer discard.Close()
	process := exec.CommandContext(ctx, executable, args...)
	process.Dir = directory
	process.Stdout = discard
	process.Stderr = discard
	process.WaitDelay = time.Second
	process.Env = append(os.Environ(), "CONFIGRA_RELEASE_DIR="+directory, "CONFIGRA_CONFIG_FILE="+filepath.Join(directory, bundle.Manifest.ConfigPath))
	if err := process.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fail(5, "release_validation_rejected")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// A formatter must not silently validate bytes different from the pinned set.
	root, err := os.OpenRoot(directory)
	if err != nil {
		return fail(5, "release_validation_modified_candidate")
	}
	defer root.Close()
	for path, value := range releaseContents(bundle) {
		actual, err := root.ReadFile(path)
		if err != nil || !bytes.Equal(actual, value) {
			clear(actual)
			return fail(5, "release_validation_modified_candidate")
		}
		clear(actual)
	}
	return nil
}
