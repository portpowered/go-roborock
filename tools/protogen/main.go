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

const (
	protocolSource = "api/external/b01_scmap.proto"
	generatedPath  = "pkg/dependencymodels/mapproto/b01_scmap.pb.go"
	generatedMode  = 0o600
)

var errGeneration = errors.New("protobuf generation failed")

func main() {
	check := flag.Bool("check", false, "verify generated output")

	flag.Parse()

	err := run(context.Background(), *check)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, check bool) error {
	input, err := compileRequest(ctx)
	if err != nil {
		return err
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

	err = proto.Unmarshal(output, response)
	if err != nil {
		return fmt.Errorf("decode plugin response: %w", err)
	}

	if response.GetError() != "" {
		return fmt.Errorf("%w: %s", errGeneration, response.GetError())
	}

	for _, file := range response.GetFile() {
		err = saveFile(file, check)
		if err != nil {
			return err
		}
	}

	return nil
}

func compileRequest(ctx context.Context) ([]byte, error) {
	resolver := new(protocompile.SourceResolver)
	resolver.ImportPaths = []string{"."}
	compiler := new(protocompile.Compiler)
	compiler.Resolver = resolver

	files, err := compiler.Compile(ctx, protocolSource)
	if err != nil {
		return nil, fmt.Errorf("compile map protocol: %w", err)
	}

	request := new(pluginpb.CodeGeneratorRequest)
	request.FileToGenerate = []string{protocolSource}

	request.Parameter = proto.String("module=github.com/portpowered/go-roborock")

	for _, file := range files {
		request.ProtoFile = append(request.ProtoFile, protodesc.ToFileDescriptorProto(file))
	}

	input, err := proto.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode plugin request: %w", err)
	}

	return input, nil
}

func saveFile(file *pluginpb.CodeGeneratorResponse_File, check bool) error {
	name := file.GetName()
	if filepath.ToSlash(filepath.Clean(name)) != generatedPath {
		return fmt.Errorf("%w: unexpected generated path %q", errGeneration, name)
	}

	if check {
		current, err := os.ReadFile(generatedPath)
		if err != nil {
			return fmt.Errorf("read generated protobuf: %w", err)
		}

		if !bytes.Equal(current, []byte(file.GetContent())) {
			return fmt.Errorf("%w: protobuf generation drift: %s", errGeneration, name)
		}

		return nil
	}

	err := os.WriteFile(generatedPath, []byte(file.GetContent()), generatedMode)
	if err != nil {
		return fmt.Errorf("write protobuf: %w", err)
	}

	return nil
}
