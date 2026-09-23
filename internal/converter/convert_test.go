package converter

import (
	"strings"
	"testing"
)

const webRedis = `
version: "3.8"
services:
  web:
    image: nginx:1.25
    ports:
      - "8080:80"
    environment:
      LOG_LEVEL: info
      APP_PORT: "80"
  redis:
    image: redis:7
    ports:
      - "6379"
`

func parseOrFail(t *testing.T, doc string) *Compose {
	t.Helper()
	c, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return c
}

func TestAcceptanceWebRedis(t *testing.T) {
	res := Convert(parseOrFail(t, webRedis))

	if len(res.Deployments) != 2 {
		t.Fatalf("want 2 deployments, got %d", len(res.Deployments))
	}
	if len(res.Services) != 2 {
		t.Fatalf("want 2 services, got %d", len(res.Services))
	}
	// Only web has environment -> exactly one ConfigMap.
	if len(res.ConfigMaps) != 1 {
		t.Fatalf("want 1 configmap, got %d", len(res.ConfigMaps))
	}
	cm := res.ConfigMaps[0]
	if got := Name(cm); got != "web-env" {
		t.Fatalf("configmap name = %q, want web-env", got)
	}
	data := cm["data"].(map[string]any)
	if data["LOG_LEVEL"] != "info" || data["APP_PORT"] != "80" {
		t.Fatalf("configmap data wrong: %v", data)
	}
}

func TestServicePortMapping(t *testing.T) {
	res := Convert(parseOrFail(t, webRedis))
	var web map[string]any
	for _, s := range res.Services {
		if Name(s) == "web" {
			web = s
		}
	}
	if web == nil {
		t.Fatal("no web service")
	}
	spec := web["spec"].(map[string]any)
	ports := spec["ports"].([]any)
	if len(ports) != 1 {
		t.Fatalf("want 1 port, got %d", len(ports))
	}
	p := ports[0].(map[string]any)
	if p["port"] != 8080 || p["targetPort"] != 80 {
		t.Fatalf("port mapping wrong: %v", p)
	}
	if spec["type"] != "ClusterIP" {
		t.Fatalf("service type = %v, want ClusterIP", spec["type"])
	}
}

func TestLabelsConsistent(t *testing.T) {
	res := Convert(parseOrFail(t, webRedis))
	dep := res.Deployments[0]
	labels := dep["metadata"].(map[string]any)["labels"].(map[string]any)
	if labels["app.kubernetes.io/managed-by"] != managedBy {
		t.Fatalf("missing managed-by label: %v", labels)
	}
	name := labels["app.kubernetes.io/name"]
	spec := dep["spec"].(map[string]any)
	sel := spec["selector"].(map[string]any)["matchLabels"].(map[string]any)
	if sel["app.kubernetes.io/name"] != name {
		t.Fatalf("selector name %v != metadata name %v", sel["app.kubernetes.io/name"], name)
	}
}

func TestReadinessStub(t *testing.T) {
	res := Convert(parseOrFail(t, webRedis))
	for _, d := range res.Deployments {
		tmpl := d["spec"].(map[string]any)["template"].(map[string]any)
		podSpec := tmpl["spec"].(map[string]any)
		c := podSpec["containers"].([]any)[0].(map[string]any)
		if _, ok := c["readinessProbe"]; !ok {
			t.Fatalf("deployment %s missing readinessProbe", Name(d))
		}
	}
}

func TestParsePorts(t *testing.T) {
	cases := map[string]Port{
		"80":              {Published: 80, Target: 80, Protocol: "TCP"},
		"8080:80":         {Published: 8080, Target: 80, Protocol: "TCP"},
		"127.0.0.1:53:53": {Published: 53, Target: 53, Protocol: "TCP"},
		"9000:9000/udp":   {Published: 9000, Target: 9000, Protocol: "UDP"},
	}
	for in, want := range cases {
		got, warns := parsePorts("t", []string{in})
		if len(warns) != 0 {
			t.Fatalf("%q: unexpected warnings %v", in, warns)
		}
		if len(got) != 1 || got[0] != want {
			t.Fatalf("%q: got %+v, want %+v", in, got, want)
		}
	}
}

func TestEnvListForm(t *testing.T) {
	doc := `
services:
  app:
    image: busybox
    environment:
      - FOO=bar
      - BAZ=qux
`
	res := Convert(parseOrFail(t, doc))
	if len(res.ConfigMaps) != 1 {
		t.Fatalf("want 1 configmap, got %d", len(res.ConfigMaps))
	}
	data := res.ConfigMaps[0]["data"].(map[string]any)
	if data["FOO"] != "bar" || data["BAZ"] != "qux" {
		t.Fatalf("env list form parsed wrong: %v", data)
	}
}

func TestUnsupportedWarnings(t *testing.T) {
	doc := `
services:
  api:
    build: .
    image: api:latest
    depends_on:
      - db
    volumes:
      - ./data:/var/lib/data
`
	res := Convert(parseOrFail(t, doc))
	joined := strings.Join(res.Warnings, "\n")
	for _, want := range []string{"build", "depends_on", "emptyDir stub"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected warning containing %q, got:\n%s", want, joined)
		}
	}
}

func TestRenderMultiDoc(t *testing.T) {
	res := Convert(parseOrFail(t, webRedis))
	out, err := Render(res)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(out)
	// ConfigMap + 2 Deployments + 2 Services = 5 docs = 4 separators.
	if n := strings.Count(s, "\n---\n"); n != 4 {
		t.Fatalf("want 4 doc separators, got %d\n%s", n, s)
	}
	if !strings.Contains(s, "kind: Deployment") || !strings.Contains(s, "kind: Service") {
		t.Fatalf("render missing kinds:\n%s", s)
	}
}

func TestParseEmpty(t *testing.T) {
	if _, err := Parse([]byte("version: \"3\"\n")); err == nil {
		t.Fatal("expected error for compose with no services")
	}
}
