// Command compose-to-k8s converts a docker-compose.yml into Kubernetes manifests.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/moveeeax/compose-to-k8s/internal/converter"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	in := flag.String("f", "docker-compose.yml", "path to the compose file")
	out := flag.String("o", "", "output directory (default: stdout as a single stream)")
	split := flag.Bool("split", false, "with -o, write one file per object instead of all.yaml")
	flag.Parse()

	data, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("read %s: %w", *in, err)
	}
	compose, err := converter.Parse(data)
	if err != nil {
		return err
	}
	res := converter.Convert(compose)

	for _, w := range res.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}

	if *out == "" {
		manifests, err := converter.Render(res)
		if err != nil {
			return err
		}
		os.Stdout.Write(manifests)
		return nil
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", *out, err)
	}

	if *split {
		return writeSplit(*out, res)
	}
	manifests, err := converter.Render(res)
	if err != nil {
		return err
	}
	dst := filepath.Join(*out, "all.yaml")
	if err := os.WriteFile(dst, manifests, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "wrote", dst)
	return nil
}

func writeSplit(dir string, res *converter.Result) error {
	write := func(kind, name string, obj map[string]any) error {
		single := &converter.Result{}
		switch kind {
		case "configmap":
			single.ConfigMaps = []map[string]any{obj}
		case "deployment":
			single.Deployments = []map[string]any{obj}
		case "service":
			single.Services = []map[string]any{obj}
		}
		b, err := converter.Render(single)
		if err != nil {
			return err
		}
		fn := filepath.Join(dir, fmt.Sprintf("%s-%s.yaml", name, kind))
		if err := os.WriteFile(fn, b, 0o644); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "wrote", fn)
		return nil
	}
	for _, cm := range res.ConfigMaps {
		if err := write("configmap", converter.Name(cm), cm); err != nil {
			return err
		}
	}
	for _, d := range res.Deployments {
		if err := write("deployment", converter.Name(d), d); err != nil {
			return err
		}
	}
	for _, s := range res.Services {
		if err := write("service", converter.Name(s), s); err != nil {
			return err
		}
	}
	return nil
}
