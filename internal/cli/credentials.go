package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	configra "github.com/viber-ops/configra-go"
)

type credentialReceipt struct {
	OperationID       string     `json:"operation_id"`
	Environment       string     `json:"environment"`
	PublicID          string     `json:"public_id,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	Fingerprint       string     `json:"certificate_fingerprint,omitempty"`
	Exported          bool       `json:"exported"`
	Server            string     `json:"server"`
	AuthorityID       string     `json:"authority_id"`
	Replaces          string     `json:"replaces,omitempty"`
	VerifyConfig      string     `json:"verify_config,omitempty"`
	Verified          bool       `json:"verified"`
	RevokeOperationID string     `json:"revoke_operation_id,omitempty"`
	Completed         bool       `json:"completed"`
}

func credentialCommand(settings *options) *cobra.Command {
	group := &cobra.Command{Use: "deployment-credential", Short: "Issue/revoke a bound read-only Token and client certificate"}
	var operation, authority, name, expiry, directory, replaces, verifyConfig string
	issue := &cobra.Command{Use: "issue", Aliases: []string{"rotate"}, Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		if command.CalledAs() == "rotate" && replaces == "" {
			return fail(2, "rotation_requires_replaced_public_id")
		}
		if replaces != "" {
			id, err := hex.DecodeString(replaces)
			if err != nil || len(id) != 8 || hex.EncodeToString(id) != replaces || !keyPattern.MatchString(verifyConfig) {
				return fail(2, "rotation_requires_valid_id_and_verify_config")
			}
		}
		if verifyConfig != "" && !keyPattern.MatchString(verifyConfig) {
			return fail(2, "invalid_verify_config")
		}
		expiresAt, err := time.Parse(time.RFC3339, expiry)
		if err != nil || !expiresAt.After(time.Now()) {
			return fail(2, "future_rfc3339_expiry_required")
		}
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			// Reserve a private destination before issuing one-time material.
			if os.Mkdir(directory, 0700) != nil {
				return fail(7, "credential_directory_must_be_new")
			}
			receipt := credentialReceipt{OperationID: operation, Environment: selected.Environment, Server: strings.TrimSuffix(selected.Server, "/"), AuthorityID: authority, Replaces: replaces, VerifyConfig: verifyConfig}
			if err := writeReceipt(directory, receipt); err != nil {
				return err
			}
			result, err := client.IssueDeploymentCredential(ctx, selected.Environment, configra.DeploymentCredentialIssue{OperationID: operation, AuthorityID: authority, DisplayName: name, ExpiresAt: expiresAt})
			if err != nil {
				return err
			}
			receipt.PublicID = result.Token.PublicID
			receipt.ExpiresAt = result.Token.ExpiresAt
			receipt.Fingerprint = result.Certificate.Certificate.FingerprintSHA256
			if err := writeReceipt(directory, receipt); err != nil {
				return err
			}
			if result.Token.Token == "" {
				_ = settings.emit(command, receipt)
				return fail(8, "one_time_export_unavailable_revoke_and_reissue")
			}
			bundle, err := base64.StdEncoding.DecodeString(result.Certificate.ExportBundle)
			if err != nil {
				return fail(8, "invalid_credential_export")
			}
			defer clear(bundle)
			files, err := credentialFiles(bundle)
			if err != nil {
				return err
			}
			files["token"] = []byte(result.Token.Token)
			files["client.zip"] = bundle
			defer func() {
				for _, value := range files {
					clear(value)
				}
			}()
			for path, value := range files {
				if err := privateOutput(command, filepath.Join(directory, path), value, false); err != nil {
					_ = settings.emit(command, receipt)
					return fail(8, "credential_export_incomplete_use_receipt_to_revoke")
				}
			}
			receipt.Exported = true
			if err := writeReceipt(directory, receipt); err != nil {
				return err
			}
			if verifyConfig != "" {
				if err := verifySavedIdentity(ctx, selected, directory, receipt); err != nil {
					_ = settings.emit(command, receipt)
					return err
				}
				receipt.Verified = true
				if err := writeReceipt(directory, receipt); err != nil {
					return err
				}
			}
			return settings.emit(command, receipt)
		})
	}}
	operationFlag(issue, &operation)
	issue.Flags().StringVar(&authority, "authority", "", "Administrator-provided managed CA ID")
	issue.Flags().StringVar(&name, "name", "", "Deployment host display name")
	issue.Flags().StringVar(&expiry, "expires-at", "", "Explicit RFC3339 expiry, bounded by writer and CA")
	issue.Flags().StringVar(&directory, "output-dir", "", "New private credential directory")
	issue.Flags().StringVar(&replaces, "replaces", "", "Old deployment public ID; rotate requires it")
	issue.Flags().StringVar(&verifyConfig, "verify-config", "", "Verify the new identity with an actual allowed Config read")
	for _, flag := range []string{"authority", "name", "expires-at", "output-dir"} {
		_ = issue.MarkFlagRequired(flag)
	}
	var revokeOperation string
	revoke := &cobra.Command{Use: "revoke PUBLIC_ID", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			if err := client.RevokeDeploymentCredential(ctx, selected.Environment, args[0], revokeOperation); err != nil {
				return err
			}
			return settings.emit(command, map[string]any{"public_id": args[0], "revoked": true})
		})
	}}
	operationFlag(revoke, &revokeOperation)
	group.AddCommand(issue, revoke)
	var after string
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, Short: "List this writer's deployment pairs in the selected Environment", RunE: func(command *cobra.Command, _ []string) error {
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			page, err := client.ListDeploymentCredentials(ctx, selected.Environment, after)
			if err != nil {
				return err
			}
			return settings.emit(command, page)
		})
	}}
	list.Flags().StringVar(&after, "after", "", "Continue after the returned public-ID cursor")
	group.AddCommand(list)
	var finishDirectory, finishOperation string
	var adopted bool
	finish := &cobra.Command{Use: "finish-rotation", Args: cobra.NoArgs, Short: "After consumer adoption, re-verify the replacement and revoke the old pair", RunE: func(command *cobra.Command, _ []string) error {
		if !adopted {
			return fail(2, "consumer_adoption_confirmation_required")
		}
		data, err := readInput(command, filepath.Join(finishDirectory, "receipt.json"), 64<<10)
		if err != nil {
			return err
		}
		var receipt credentialReceipt
		if safeJSON(data, &receipt) != nil || !receipt.Exported || !receipt.Verified || receipt.Replaces == "" || !keyPattern.MatchString(receipt.VerifyConfig) {
			return fail(2, "verified_rotation_receipt_required")
		}
		if receipt.RevokeOperationID != "" && receipt.RevokeOperationID != finishOperation {
			return fail(2, "rotation_operation_id_changed")
		}
		return settings.network(command, func(ctx context.Context, client *configra.Client, selected profile) error {
			if err := verifySavedIdentity(ctx, selected, finishDirectory, receipt); err != nil {
				return err
			}
			receipt.RevokeOperationID = finishOperation
			if err := writeReceipt(finishDirectory, receipt); err != nil {
				return err
			}
			if err := client.RevokeDeploymentCredential(ctx, selected.Environment, receipt.Replaces, finishOperation); err != nil {
				return err
			}
			receipt.Completed = true
			if err := writeReceipt(finishDirectory, receipt); err != nil {
				return err
			}
			return settings.emit(command, receipt)
		})
	}}
	finish.Flags().StringVar(&finishDirectory, "directory", "", "Replacement identity directory and durable receipt")
	finish.Flags().BoolVar(&adopted, "consumer-confirmed", false, "The deployment system verified that actual consumers adopted the replacement")
	_ = finish.MarkFlagRequired("directory")
	operationFlag(finish, &finishOperation)
	group.AddCommand(finish)
	return group
}

func verifySavedIdentity(ctx context.Context, selected profile, directory string, receipt credentialReceipt) error {
	if receipt.Server != strings.TrimSuffix(selected.Server, "/") || receipt.Environment != selected.Environment {
		return fail(2, "rotation_target_mismatch")
	}
	client, err := configra.NewClient(configra.ClientOptions{BaseURL: selected.Server, TokenFile: filepath.Join(directory, "token"), ClientCertificateFile: filepath.Join(directory, "client.crt"), ClientKeyFile: filepath.Join(directory, "client.key"), ServerCAFile: selected.ServerCAFile})
	if err != nil {
		return fail(2, "invalid_saved_identity")
	}
	defer client.CloseIdleConnections()
	self, err := client.ReadIdentity(ctx, selected.Environment)
	if err != nil {
		return err
	}
	if self.PublicID != receipt.PublicID || self.Kind != "read-only" || self.CertificateFingerprint != receipt.Fingerprint {
		return fail(2, "credential_receipt_mismatch")
	}
	_, err = client.ReadResolvedConfig(ctx, selected.Environment, receipt.VerifyConfig, "")
	return err
}

func writeReceipt(directory string, receipt credentialReceipt) error {
	file, err := os.CreateTemp(directory, ".receipt-*")
	if err != nil {
		return fail(7, "cannot_save_credential_receipt")
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(receipt)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err != nil || syncErr != nil || closeErr != nil || os.Rename(file.Name(), filepath.Join(directory, "receipt.json")) != nil {
		return fail(7, "cannot_save_credential_receipt")
	}
	return nil
}

func credentialFiles(bundle []byte) (map[string][]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil || len(archive.File) != 3 {
		return nil, fail(8, "invalid_credential_export")
	}
	result := map[string][]byte{}
	for _, file := range archive.File {
		if (file.Name != "client.crt" && file.Name != "client.key" && file.Name != "ca.crt") || !file.Mode().IsRegular() || result[file.Name] != nil {
			return nil, fail(8, "invalid_credential_export")
		}
		reader, err := file.Open()
		if err != nil {
			return nil, fail(8, "invalid_credential_export")
		}
		value, err := io.ReadAll(io.LimitReader(reader, 65537))
		_ = reader.Close()
		if err != nil || len(value) == 0 || len(value) > 65536 {
			return nil, fail(8, "invalid_credential_export")
		}
		result[file.Name] = value
	}
	return result, nil
}
