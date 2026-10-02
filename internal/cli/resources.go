package cli

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/spf13/cobra"
	configra "github.com/viber-ops/configra-go"
)

func configCommand(settings *options) *cobra.Command {
	group := &cobra.Command{Use: "config", Short: "Read and write versioned Config documents"}
	var output string
	var reveal, raw bool
	get := &cobra.Command{Use: "get NAME", Args: resourceArgs(1), RunE: func(command *cobra.Command, args []string) error {
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			var content string
			var revision uint64
			if raw {
				value, err := client.ReadRawConfig(ctx, selected.Environment, args[0])
				if err != nil {
					return err
				}
				content, revision = value.Content, value.Revision
			} else {
				value, err := client.ReadResolvedConfig(ctx, selected.Environment, args[0], "")
				if err != nil {
					return err
				}
				content, revision = value.Content, value.ConfigRevision
			}
			if err := privateOutput(command, output, []byte(content), reveal); err != nil {
				return err
			}
			if output == "-" {
				return nil
			}
			return settings.emit(command, map[string]any{"revision": revision, "written": true})
		})
	}}
	outputFlags(get, &output, &reveal)
	get.Flags().BoolVar(&raw, "raw", false, "Download canonical source (requires a scoped writer)")
	var input, name, format string
	var revision configra.RevisionWrite
	put := &cobra.Command{Use: "put NAME", Args: resourceArgs(1), RunE: func(command *cobra.Command, args []string) error {
		content, err := readInput(command, input, 5<<20)
		if err != nil {
			return err
		}
		defer clear(content)
		if name == "" {
			name = args[0]
		}
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			result, err := client.WriteConfig(ctx, selected.Environment, args[0], configra.ConfigWrite{RevisionWrite: revision, Name: name, Format: format, Content: string(content)})
			if err != nil {
				return err
			}
			return settings.emit(command, result)
		})
	}}
	fileFlag(put, &input)
	writeFlags(put, &revision)
	put.Flags().StringVar(&name, "name", "", "Display name (defaults to the Config key)")
	put.Flags().StringVar(&format, "format", "yaml", "yaml or json")
	group.AddCommand(get, put)
	return group
}

func vaultCommand(settings *options) *cobra.Command {
	group := &cobra.Command{Use: "vault", Short: "Scoped Vault Item, Field and File operations"}
	item := &cobra.Command{Use: "item", Short: "Read or replace one Environment's complete Item snapshot"}
	field := &cobra.Command{Use: "field", Short: "Write or archive a Field"}
	file := &cobra.Command{Use: "file", Short: "Upload or download binary File content"}
	group.AddCommand(item, field, file)
	var output string
	var reveal bool
	get := &cobra.Command{Use: "get NAMESPACE ITEM", Args: resourceArgs(2), RunE: func(command *cobra.Command, args []string) error {
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			value, err := client.ReadVaultItem(ctx, selected.Environment, args[0], args[1])
			if err != nil {
				return err
			}
			encoded, err := json.MarshalIndent(value, "", "  ")
			if err != nil {
				return fail(7, "cannot_encode_output")
			}
			defer clear(encoded)
			if err := privateOutput(command, output, encoded, reveal); err != nil {
				return err
			}
			if output == "-" {
				return nil
			}
			return settings.emit(command, map[string]any{"revision": value.Revision, "written": true})
		})
	}}
	outputFlags(get, &output, &reveal)
	item.AddCommand(get)
	var input string
	var revision configra.RevisionWrite
	put := &cobra.Command{Use: "put NAMESPACE ITEM", Args: resourceArgs(2), RunE: func(command *cobra.Command, args []string) error {
		encoded, err := readInput(command, input, 32<<20)
		if err != nil {
			return err
		}
		defer clear(encoded)
		var value configra.VaultItemWrite
		if err := safeJSON(encoded, &value); err != nil {
			return err
		}
		value.RevisionWrite = revision
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			result, err := client.WriteVaultItem(ctx, selected.Environment, args[0], args[1], value)
			if err != nil {
				return err
			}
			return settings.emit(command, result)
		})
	}}
	fileFlag(put, &input)
	writeFlags(put, &revision)
	item.AddCommand(put)
	for _, entry := range []struct {
		group *cobra.Command
		kind  string
		count int
	}{{item, "item", 2}, {field, "field", 3}} {
		entry := entry
		var version configra.RevisionWrite
		remove := &cobra.Command{Use: "delete NAMESPACE ITEM [FIELD]", Short: "Archive with retained history; shared changes require every bound Environment", Args: resourceArgs(entry.count), RunE: func(command *cobra.Command, args []string) error {
			return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
				var result configra.MutationResult
				var err error
				if entry.kind == "item" {
					result, err = client.DeleteVaultItem(ctx, selected.Environment, args[0], args[1], version)
				} else {
					result, err = client.DeleteVaultField(ctx, selected.Environment, args[0], args[1], args[2], version)
				}
				if err != nil {
					return err
				}
				return settings.emit(command, result)
			})
		}}
		writeFlags(remove, &version)
		entry.group.AddCommand(remove)
	}
	for _, binary := range []bool{false, true} {
		var input, name, kind, filename, contentType string
		var version configra.RevisionWrite
		put := &cobra.Command{Use: "put NAMESPACE ITEM FIELD", Args: resourceArgs(3), RunE: func(command *cobra.Command, args []string) error {
			data, err := readInput(command, input, 5<<20)
			if err != nil {
				return err
			}
			defer clear(data)
			if name == "" {
				name = args[2]
			}
			return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
				var result configra.MutationResult
				var err error
				if binary {
					if filename == "" {
						if input == "-" {
							filename = args[2]
						} else {
							filename = filepath.Base(input)
						}
					}
					result, err = client.WriteFile(ctx, selected.Environment, args[0], args[1], args[2], configra.FileWrite{RevisionWrite: version, Name: name, File: configra.VaultFile{Filename: filename, ContentType: contentType, Bytes: data}})
				} else {
					if kind != "text" && kind != "secret" {
						return fail(2, "field_type_must_be_text_or_secret")
					}
					value := string(data)
					result, err = client.WriteVaultField(ctx, selected.Environment, args[0], args[1], args[2], configra.VaultFieldWrite{RevisionWrite: version, Name: name, Type: kind, Value: configra.VaultValue{Text: &value}})
				}
				if err != nil {
					return err
				}
				return settings.emit(command, result)
			})
		}}
		fileFlag(put, &input)
		writeFlags(put, &version)
		put.Flags().StringVar(&name, "name", "", "Display name (defaults to Field key)")
		if binary {
			put.Flags().StringVar(&filename, "filename", "", "Stored filename (defaults to input basename)")
			put.Flags().StringVar(&contentType, "content-type", "application/octet-stream", "Stored MIME type")
			file.AddCommand(put)
		} else {
			put.Flags().StringVar(&kind, "type", "secret", "text or secret")
			field.AddCommand(put)
		}
	}
	var fileOutput string
	var fileReveal bool
	fileGet := &cobra.Command{Use: "get NAMESPACE ITEM FIELD", Args: resourceArgs(3), RunE: func(command *cobra.Command, args []string) error {
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			value, err := client.ReadFile(ctx, selected.Environment, args[0], args[1], args[2], "")
			if err != nil {
				return err
			}
			defer clear(value.Bytes)
			if err := privateOutput(command, fileOutput, value.Bytes, fileReveal); err != nil {
				return err
			}
			if fileOutput == "-" {
				return nil
			}
			return settings.emit(command, map[string]any{"written": true, "bytes": len(value.Bytes)})
		})
	}}
	outputFlags(fileGet, &fileOutput, &fileReveal)
	file.AddCommand(fileGet)
	return group
}
