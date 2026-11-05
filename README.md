# compose-to-k8s

[![CI](https://github.com/moveeeax/compose-to-k8s/actions/workflows/ci.yml/badge.svg)](https://github.com/moveeeax/compose-to-k8s/actions/workflows/ci.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/moveeeax/compose-to-k8s)](go.mod)
[![Go Reference](https://pkg.go.dev/badge/github.com/moveeeax/compose-to-k8s.svg)](https://pkg.go.dev/github.com/moveeeax/compose-to-k8s)
[![Go Report Card](https://goreportcard.com/badge/github.com/moveeeax/compose-to-k8s)](https://goreportcard.com/report/github.com/moveeeax/compose-to-k8s)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Turn a `docker-compose.yml` into idiomatic Kubernetes manifests — a Deployment, a
Service, and a ConfigMap per compose service, with consistent
`app.kubernetes.io/*` labels and readiness stubs.

`kompose` is effectively unmaintained and its output needs hand-cleaning. This
tool does the common 80% case cleanly and tells you exactly what it couldn't
translate instead of guessing.

## What it maps

| compose | Kubernetes |
| --- | --- |
| `services.<name>` | a `Deployment` (`replicas: 1`) |
| `ports` | a `ClusterIP` `Service` + `containerPort`s + a TCP readiness probe |
| `environment` (map **or** `KEY=value` list) | a `ConfigMap`, wired via `envFrom` |
| `command` (string or list) | container `command` |
| `volumes` | `emptyDir` stubs + a warning to wire a real PVC |

Unsupported keys (`build`, `depends_on`, `deploy`) produce a warning on stderr
rather than silently wrong YAML. Output ordering and label sets are
deterministic, so re-running produces a clean diff.

## Install

Requires Go 1.26 or newer (CI tests Go 1.26 and 1.27).

```sh
go install github.com/moveeeax/compose-to-k8s/cmd/compose-to-k8s@latest
```

Or build from source:

```sh
git clone https://github.com/moveeeax/compose-to-k8s
cd compose-to-k8s
go build ./cmd/compose-to-k8s
```

## Usage

```sh
# Print a single multi-doc stream to stdout
compose-to-k8s -f docker-compose.yml

# Write out/all.yaml
compose-to-k8s -f docker-compose.yml -o ./out

# Write one file per object (web-deployment.yaml, web-service.yaml, ...)
compose-to-k8s -f docker-compose.yml -o ./out -split
```

Example (`examples/simple/docker-compose.yml`):

```sh
$ compose-to-k8s -f examples/simple/docker-compose.yml -o ./out
warning: service "redis": volume "redis-data:/data" emitted as an emptyDir stub; wire a real PVC/hostPath before production
wrote out/all.yaml

$ kubeconform -strict -summary out/all.yaml
Summary: 5 resources found in 1 file - Valid: 5, Invalid: 0, Errors: 0, Skipped: 0
```

The generated manifests pass `kubeconform -strict` and apply cleanly with
`kubectl apply -f out/`.

## Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `-f` | `docker-compose.yml` | input compose file |
| `-o` | *(stdout)* | output directory |
| `-split` | `false` | with `-o`, one file per object |

## Development

```sh
go test -race ./...  # unit tests, including the web+redis acceptance case
go vet ./...
```

## License

MIT — see [LICENSE](LICENSE).

---

**Archived on 2026-11-05.** the README's premise that kompose is "effectively unmaintained" no longer holds (kubernetes/kompose v1.38.0, Jan 2026), and kompose already covers probes, PVCs, service types and Ingress via labels; this ~600-line converter handles image/ports/env/command/volumes only. Keep only if the "clean output + honest warnings, no kompose labels" angle is pursued with the features below.
