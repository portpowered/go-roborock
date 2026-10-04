// Command protogen compiles the pinned map protocol with the official Go plugin.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	check := flag.Bool("check", false, "verify generated output")
	flag.Parse()
	if err := run(context.Background(), *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, check bool) error {
	const source = "api/external/b01_scmap.proto"
	compiler := protocompile.Compiler{Resolver: &protocompile.SourceResolver{ImportPaths: []string{"."}}}
	files, err := compiler.Compile(ctx, source)
	if err != nil {
		return fmt.Errorf("compile map protocol: %w", err)
	}
	request := &pluginpb.CodeGeneratorRequest{FileToGenerate: []string{source}, Parameter: proto.String("module=github.com/portpowered/go-roborock")}
	for _, file := range files {
		request.ProtoFile = append(request.ProtoFile, protodesc.ToFileDescriptorProto(file))
	}
	input, err := proto.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode plugin request: %w", err)
	}
	command := exec.CommandContext(ctx, "go", "run", "google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11")
	command.Stdin = bytes.NewReader(input)
	var diagnostics bytes.Buffer
	command.Stderr = &diagnostics
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("generate protobuf: %w: %s", err, diagnostics.String())
	}
	response := new(pluginpb.CodeGeneratorResponse)
	if err = proto.Unmarshal(output, response); err != nil {
		return fmt.Errorf("decode plugin response: %w", err)
	}
	if response.GetError() != "" {
		return errors.New(response.GetError())
	}
	for _, file := range response.GetFile() {
		name := file.GetName()
		if filepath.Clean(name) != "pkg/dependencymodels/mapproto/b01_scmap.pb.go" && filepath.ToSlash(filepath.Clean(name)) != "pkg/dependencymodels/mapproto/b01_scmap.pb.go" {
			return fmt.Errorf("unexpected generated path %q", name)
		}
		if check {
			current, readErr := os.ReadFile(name)
			if readErr != nil || !bytes.Equal(current, []byte(file.GetContent())) {
				return fmt.Errorf("protobuf generation drift: %s", name)
			}
		} else if err = os.WriteFile(name, []byte(file.GetContent()), 0o600); err != nil {
			return fmt.Errorf("write protobuf: %w", err)
		}
	}
	return nil
}
