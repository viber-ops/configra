package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

type profile struct {
	Server          string `json:"server" yaml:"server"`
	Environment     string `json:"environment" yaml:"environment"`
	TokenFile       string `json:"token_file" yaml:"token_file"`
	CertificateFile string `json:"certificate_file,omitempty" yaml:"certificate_file,omitempty"`
	KeyFile         string `json:"key_file,omitempty" yaml:"key_file,omitempty"`
	ServerCAFile    string `json:"server_ca_file,omitempty" yaml:"server_ca_file,omitempty"`
}

type profiles struct {
	Current  string             `json:"current,omitempty" yaml:"current,omitempty"`
	Contexts map[string]profile `json:"contexts" yaml:"contexts"`
}

func loadProfiles(path string) (profiles, error) {
	result := profiles{Contexts: map[string]profile{}}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, fail(7, "cannot_read_context_file")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return result, fail(2, "invalid_context_file")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.Contexts == nil {
		return result, fail(2, "invalid_context_file")
	}
	return result, nil
}

func saveProfiles(path string, values profiles) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fail(7, "context_file_must_be_regular")
	}
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return fail(7, "cannot_create_context_directory")
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".configractl-*")
	if err != nil {
		return fail(7, "cannot_save_context")
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(values)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err != nil || syncErr != nil || closeErr != nil || os.Rename(file.Name(), path) != nil {
		return fail(7, "cannot_save_context")
	}
	return nil
}

func environmentProfile() profile {
	return profile{Server: os.Getenv("CONFIGRA_URL"), Environment: os.Getenv("CONFIGRA_ENVIRONMENT"), TokenFile: os.Getenv("CONFIGRA_TOKEN_FILE"), CertificateFile: os.Getenv("CONFIGRA_CLIENT_CERT"), KeyFile: os.Getenv("CONFIGRA_CLIENT_KEY"), ServerCAFile: os.Getenv("CONFIGRA_SERVER_CA")}
}

func (settings *options) overrides(command *cobra.Command, selected profile) profile {
	for _, pair := range []struct {
		flag   string
		target *string
		value  string
	}{
		{"server", &selected.Server, settings.Server}, {"environment", &selected.Environment, settings.Environment}, {"token-file", &selected.TokenFile, settings.TokenFile},
		{"client-cert", &selected.CertificateFile, settings.CertificateFile}, {"client-key", &selected.KeyFile, settings.KeyFile}, {"server-ca", &selected.ServerCAFile, settings.ServerCAFile},
	} {
		if command.Flags().Changed(pair.flag) {
			*pair.target = pair.value
		}
	}
	return selected
}

func (settings *options) connection(command *cobra.Command) (profile, error) {
	values, err := loadProfiles(settings.contextFile)
	if err != nil {
		return profile{}, err
	}
	name := settings.contextName
	if name == "" {
		name = values.Current
	}
	selected := environmentProfile()
	if name != "" {
		var found bool
		selected, found = values.Contexts[name]
		if !found {
			return profile{}, fail(2, "context_not_found")
		}
	}
	return settings.overrides(command, selected), nil
}

func contextCommand(settings *options) *cobra.Command {
	group := &cobra.Command{Use: "context", Short: "Store named API targets and credential file paths"}
	group.AddCommand(&cobra.Command{Use: "set NAME", Args: resourceArgs(1), RunE: func(command *cobra.Command, args []string) error {
		selected := settings.overrides(command, environmentProfile())
		parsed, err := url.Parse(selected.Server)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") || !keyPattern.MatchString(selected.Environment) || selected.TokenFile == "" {
			return fail(2, "context_requires_https_server_environment_and_token_file")
		}
		for _, path := range []*string{&selected.TokenFile, &selected.CertificateFile, &selected.KeyFile, &selected.ServerCAFile} {
			if *path != "" {
				absolute, err := filepath.Abs(*path)
				if err != nil {
					return fail(2, "invalid_credential_path")
				}
				*path = absolute
			}
		}
		values, err := loadProfiles(settings.contextFile)
		if err != nil {
			return err
		}
		values.Contexts[args[0]] = selected
		if err := saveProfiles(settings.contextFile, values); err != nil {
			return err
		}
		return settings.emit(command, map[string]any{"context": args[0], "saved": true})
	}})
	group.AddCommand(&cobra.Command{Use: "use NAME", Args: resourceArgs(1), RunE: func(command *cobra.Command, args []string) error {
		values, err := loadProfiles(settings.contextFile)
		if err != nil {
			return err
		}
		if _, ok := values.Contexts[args[0]]; !ok {
			return fail(2, "context_not_found")
		}
		values.Current = args[0]
		if err := saveProfiles(settings.contextFile, values); err != nil {
			return err
		}
		return settings.emit(command, map[string]any{"context": args[0]})
	}})
	group.AddCommand(&cobra.Command{Use: "show", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		values, err := loadProfiles(settings.contextFile)
		if err != nil {
			return err
		}
		return settings.emit(command, values)
	}})
	return group
}
