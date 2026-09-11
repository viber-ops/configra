package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "sigs.k8s.io/secrets-store-csi-driver/provider/v1alpha1"

	"github.com/viber-ops/configra/kubernetes/internal/source"
)

type Server struct {
	pb.UnimplementedCSIDriverProviderServer
	Source source.Fetcher
}

func (server *Server) Version(context.Context, *pb.VersionRequest) (*pb.VersionResponse, error) {
	return &pb.VersionResponse{Version: "v1alpha1", RuntimeName: "configra", RuntimeVersion: "0.1.0"}, nil
}

func (server *Server) Mount(ctx context.Context, request *pb.MountRequest) (*pb.MountResponse, error) {
	if request == nil || len(request.Attributes) > 64<<10 || len(request.Secrets) > 160<<10 || server.Source == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid mount request")
	}
	var attributes, secrets map[string]string
	if json.Unmarshal([]byte(request.Attributes), &attributes) != nil || json.Unmarshal([]byte(request.Secrets), &secrets) != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid mount attributes or credentials")
	}
	// The origin and server trust are deployment configuration, never SPC parameters.
	for _, forbidden := range []string{"url", "baseURL", "base_url", "configraURL", "configra_url", "serverCA", "insecureSkipVerify"} {
		if _, exists := attributes[forbidden]; exists {
			return nil, status.Error(codes.InvalidArgument, "upstream origin and trust cannot be overridden")
		}
	}
	var objects []source.Object
	mode := int32(0440)
	switch attributes["fileMode"] {
	case "", "0440":
	case "0400":
		mode = 0400
	case "0444":
		mode = 0444
	default:
		return nil, status.Error(codes.InvalidArgument, "fileMode must be 0400, 0440, or 0444")
	}
	decoder := yaml.NewDecoder(strings.NewReader(attributes["objects"]))
	decoder.KnownFields(true)
	if decoder.Decode(&objects) != nil || source.ValidateObjects(objects) != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid object mapping")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, status.Error(codes.InvalidArgument, "object mapping must contain one document")
	}
	credentials := map[string][]byte{}
	for _, name := range []string{"token", "tls.crt", "tls.key"} {
		credentials[name] = []byte(secrets[name])
	}
	defer func() {
		for _, value := range credentials {
			clear(value)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	materials, err := server.Source.Read(ctx, objects, credentials)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "Configra objects could not be fetched")
	}
	if len(materials) != len(objects) {
		return nil, status.Error(codes.Internal, "incomplete Configra response")
	}
	result := &pb.MountResponse{}
	total := 0
	for index, material := range materials {
		total += len(material.Bytes)
		if material.Path != objects[index].Path || material.Version == "" || total > source.MaxContentBytes {
			return nil, status.Error(codes.Internal, "invalid Configra response")
		}
		result.Files = append(result.Files, &pb.File{Path: material.Path, Mode: mode, Contents: material.Bytes})
		result.ObjectVersion = append(result.ObjectVersion, &pb.ObjectVersion{Id: material.Path, Version: material.Version})
	}
	return result, nil
}
