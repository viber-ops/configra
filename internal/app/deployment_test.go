package app

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionDockerfileIsStaticNonRootAndToolchainPinned(t *testing.T) {
	encoded, err := os.ReadFile(filepath.Join("..", "..", "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	dockerfile := string(encoded)
	for _, required := range []string{
		"golang:1.26.7-bookworm", "CGO_ENABLED=0", "FROM scratch", "ca-certificates.crt",
		"USER 65532:65532", `ENTRYPOINT ["/configra"]`,
	} {
		if !strings.Contains(dockerfile, required) {
			t.Errorf("Dockerfile is missing %q", required)
		}
	}
	for _, forbidden := range []string{"latest", "apt-get", "apk add", "InsecureSkipVerify"} {
		if strings.Contains(dockerfile, forbidden) {
			t.Errorf("Dockerfile contains forbidden %q", forbidden)
		}
	}
}

func TestKubernetesBaseBuildsSeparatedHardenedDeployments(t *testing.T) {
	kubectl, err := exec.LookPath("kubectl")
	if err != nil {
		t.Skip("kubectl is not installed")
	}
	command := exec.Command(kubectl, "kustomize", filepath.Join("..", "..", "deploy", "kubernetes", "base"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl kustomize: %v\n%s", err, output)
	}
	for _, required := range [][]byte{
		[]byte("name: configra-management"), []byte("name: configra-api"),
		[]byte("automountServiceAccountToken: false"), []byte("runAsNonRoot: true"),
		[]byte("readOnlyRootFilesystem: true"), []byte("allowPrivilegeEscalation: false"),
		[]byte("seccompProfile:"), []byte("startupProbe:"), []byte("readinessProbe:"),
	} {
		if !bytes.Contains(output, required) {
			t.Errorf("rendered Kubernetes base is missing %q", required)
		}
	}
	if count := bytes.Count(output, []byte("kind: Deployment")); count != 2 {
		t.Errorf("Deployment count = %d, want 2", count)
	}
	if count := bytes.Count(output, []byte("kind: Service")); count != 2 {
		t.Errorf("Service count = %d, want 2", count)
	}
}

func TestBackupToolsRejectUnsafeDatabaseNamesBeforeConnecting(t *testing.T) {
	repository := filepath.Join("..", "..")
	tests := []struct {
		script    string
		arguments []string
	}{
		{"mysql-backup.sh", []string{"missing.cnf", "configra;drop", "missing"}},
		{"mysql-restore.sh", []string{"missing.cnf", "missing", "configra;drop"}},
		{"clickhouse-backup.sh", []string{"missing.xml", "configra;drop", "backups", "backup.zip"}},
		{"clickhouse-restore.sh", []string{"missing.xml", "configra", "backups", "backup.zip", "configra;drop"}},
	}
	for _, test := range tests {
		t.Run(test.script, func(t *testing.T) {
			arguments := append([]string{filepath.Join(repository, "deploy", "backup", test.script)}, test.arguments...)
			output, err := exec.Command("sh", arguments...).CombinedOutput()
			if err == nil {
				t.Fatal("backup tool accepted an unsafe database name")
			}
			if bytes.Contains(output, []byte("configra;drop")) {
				t.Fatal("backup tool reflected an unsafe database name")
			}
		})
	}
}
