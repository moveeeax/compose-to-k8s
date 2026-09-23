package converter

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// AppLabel is the name given to the whole app for the managed-by/instance labels.
const managedBy = "compose-to-k8s"

// Result holds the generated manifests and any warnings raised while converting.
type Result struct {
	Deployments []map[string]any
	Services    []map[string]any
	ConfigMaps  []map[string]any
	Warnings    []string
}

// Port is a parsed compose port mapping.
type Port struct {
	Published int
	Target    int
	Protocol  string
}

// Convert turns a parsed compose spec into k8s objects.
func Convert(c *Compose) *Result {
	res := &Result{}
	for _, name := range c.ServiceNames() {
		svc := c.Services[name]
		labels := commonLabels(name)

		res.Warnings = append(res.Warnings, warnUnsupported(name, svc)...)

		ports, portWarns := parsePorts(name, svc.Ports)
		res.Warnings = append(res.Warnings, portWarns...)

		var cmName string
		if len(svc.Environment) > 0 {
			cm := buildConfigMap(name, labels, svc.Environment)
			res.ConfigMaps = append(res.ConfigMaps, cm)
			cmName = name + "-env"
		}

		dep := buildDeployment(name, labels, svc, ports, cmName)
		res.Deployments = append(res.Deployments, dep)

		if len(ports) > 0 {
			res.Services = append(res.Services, buildService(name, labels, ports))
		}
	}
	return res
}

func commonLabels(name string) map[string]any {
	return map[string]any{
		"app.kubernetes.io/name":       name,
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/managed-by": managedBy,
	}
}

func warnUnsupported(name string, svc Service) []string {
	var w []string
	if svc.Build != nil {
		w = append(w, fmt.Sprintf("service %q: 'build' is not supported; set a prebuilt 'image' instead", name))
	}
	if svc.DependsOn != nil {
		w = append(w, fmt.Sprintf("service %q: 'depends_on' has no direct k8s equivalent; use readiness probes/init logic", name))
	}
	if svc.Deploy != nil {
		w = append(w, fmt.Sprintf("service %q: 'deploy' block is ignored (replicas/resources not translated)", name))
	}
	for _, v := range svc.Volumes {
		w = append(w, fmt.Sprintf("service %q: volume %q emitted as an emptyDir stub; wire a real PVC/hostPath before production", name, v))
	}
	return w
}

// parsePorts understands "8080", "8080:80", "127.0.0.1:8080:80" and "80/udp".
func parsePorts(name string, raw []string) ([]Port, []string) {
	var ports []Port
	var warns []string
	for _, p := range raw {
		proto := "TCP"
		spec := p
		if i := strings.Index(spec, "/"); i >= 0 {
			proto = strings.ToUpper(spec[i+1:])
			spec = spec[:i]
		}
		parts := strings.Split(spec, ":")
		var pubStr, tgtStr string
		switch len(parts) {
		case 1:
			pubStr, tgtStr = parts[0], parts[0]
		case 2:
			pubStr, tgtStr = parts[0], parts[1]
		case 3: // host-ip:published:target
			pubStr, tgtStr = parts[1], parts[2]
		default:
			warns = append(warns, fmt.Sprintf("service %q: cannot parse port %q; skipped", name, p))
			continue
		}
		pub, err1 := strconv.Atoi(pubStr)
		tgt, err2 := strconv.Atoi(tgtStr)
		if err1 != nil || err2 != nil {
			warns = append(warns, fmt.Sprintf("service %q: port %q has a non-numeric value; skipped", name, p))
			continue
		}
		ports = append(ports, Port{Published: pub, Target: tgt, Protocol: proto})
	}
	return ports, warns
}

func buildConfigMap(name string, labels map[string]any, env Environment) map[string]any {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	data := map[string]any{}
	for _, k := range keys {
		data[k] = env[k]
	}
	return map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":   name + "-env",
			"labels": labels,
		},
		"data": data,
	}
}

func buildDeployment(name string, labels map[string]any, svc Service, ports []Port, cmName string) map[string]any {
	container := map[string]any{
		"name":  name,
		"image": svc.Image,
	}
	if len(svc.Command) > 0 {
		container["command"] = toAnySlice(svc.Command)
	}
	if len(ports) > 0 {
		var cp []any
		for _, p := range ports {
			cp = append(cp, map[string]any{
				"containerPort": p.Target,
				"protocol":      p.Protocol,
			})
		}
		container["ports"] = cp
		// readiness stub against the first port
		container["readinessProbe"] = map[string]any{
			"tcpSocket":           map[string]any{"port": ports[0].Target},
			"initialDelaySeconds": 5,
			"periodSeconds":       10,
		}
	}
	if cmName != "" {
		container["envFrom"] = []any{
			map[string]any{"configMapRef": map[string]any{"name": cmName}},
		}
	}
	if vols := volumeMounts(svc.Volumes); vols != nil {
		container["volumeMounts"] = vols
	}

	podSpec := map[string]any{"containers": []any{container}}
	if v := podVolumes(svc.Volumes); v != nil {
		podSpec["volumes"] = v
	}

	selector := map[string]any{
		"app.kubernetes.io/name":     name,
		"app.kubernetes.io/instance": name,
	}
	return map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":   name,
			"labels": labels,
		},
		"spec": map[string]any{
			"replicas": 1,
			"selector": map[string]any{"matchLabels": selector},
			"template": map[string]any{
				"metadata": map[string]any{"labels": labels},
				"spec":     podSpec,
			},
		},
	}
}

func buildService(name string, labels map[string]any, ports []Port) map[string]any {
	var sp []any
	for _, p := range ports {
		sp = append(sp, map[string]any{
			"name":       fmt.Sprintf("port-%d", p.Published),
			"port":       p.Published,
			"targetPort": p.Target,
			"protocol":   p.Protocol,
		})
	}
	selector := map[string]any{
		"app.kubernetes.io/name":     name,
		"app.kubernetes.io/instance": name,
	}
	return map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]any{
			"name":   name,
			"labels": labels,
		},
		"spec": map[string]any{
			"type":     "ClusterIP",
			"selector": selector,
			"ports":    sp,
		},
	}
}

// volumeMounts converts compose "src:dst[:mode]" strings into emptyDir mounts.
func volumeMounts(vols []string) []any {
	var out []any
	for i, v := range vols {
		_, dst := splitVolume(v)
		out = append(out, map[string]any{
			"name":      volName(i),
			"mountPath": dst,
		})
	}
	return out
}

func podVolumes(vols []string) []any {
	var out []any
	for i := range vols {
		out = append(out, map[string]any{
			"name":     volName(i),
			"emptyDir": map[string]any{},
		})
	}
	return out
}

func volName(i int) string { return fmt.Sprintf("vol-%d", i) }

func splitVolume(v string) (string, string) {
	parts := strings.Split(v, ":")
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[0], parts[1]
}

func toAnySlice(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
