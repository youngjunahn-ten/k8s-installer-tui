package schema

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// testdata/vars.schema.json is ansible/schema/vars.schema.json as
// tools/export-vars-schema.py of k8s-installer writes it (branch
// feat/export-vars-schema, on main b8a1bd6).

func load(t *testing.T) *Schema {
	t.Helper()
	s, err := Load(filepath.Join("testdata", "vars.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFieldsFollowVarsYml(t *testing.T) {
	s := load(t)
	if len(s.Fields) != 63 {
		t.Fatalf("%d fields, want the 63 variables of vars.yml", len(s.Fields))
	}
	if s.Fields[0].Name != "ansible_user" || s.Fields[1].Name != "ki_var_root_path" {
		t.Errorf("order starts %s, %s; want the order of vars.yml", s.Fields[0].Name, s.Fields[1].Name)
	}
}

func TestEveryFieldHasAnInput(t *testing.T) {
	count := map[Kind]int{}
	for _, f := range load(t).Fields {
		if f.Kind == "" {
			t.Errorf("%s has no kind", f.Name)
		}
		count[f.Kind]++
	}
	// All six kinds are in use, so a form needs an input for each.
	for _, k := range []Kind{KindBool, KindInt, KindString, KindEnum, KindList, KindObject} {
		if count[k] == 0 {
			t.Errorf("no field of kind %s", k)
		}
	}
	t.Logf("kinds: %v", count)
}

func TestWhatAFormNeedsIsThere(t *testing.T) {
	s := load(t)

	carrier, _ := s.Field("ki_cp_ha_mode_vip_carrier")
	if carrier.Kind != KindEnum || !reflect.DeepEqual(carrier.Enum, []string{"arp", "aws-eni"}) || carrier.Default != "arp" {
		t.Errorf("carrier: %+v", carrier)
	}
	if !strings.HasPrefix(carrier.Summary(), "Who tells the network where the vip is.") || strings.Contains(carrier.Summary(), "\n") {
		t.Errorf("summary %q", carrier.Summary())
	}
	if carrier.Class != "service" {
		t.Errorf("class %q", carrier.Class)
	}

	vip, _ := s.Field("ki_cp_ha_mode_vip")
	if vip.Kind != KindString || !vip.Nullable || vip.Format != "ipv4" || vip.Required {
		t.Errorf("vip: %+v", vip)
	}

	subnets, _ := s.Field("internal_network_subnets")
	if subnets.Kind != KindList || !subnets.Required {
		t.Errorf("subnets: %+v", subnets)
	}

	maxPods, _ := s.Field("kubelet_max_pods")
	if maxPods.Kind != KindInt || !maxPods.Nullable {
		t.Errorf("max pods: %+v", maxPods)
	}

	zone, _ := s.Field("internal_network_extra_zone")
	if !zone.Commented || zone.Example != "example.com" {
		t.Errorf("zone: %+v", zone)
	}

	proxy, _ := s.Field("k8s_kube_proxy_enabled")
	if proxy.Kind != KindBool || !proxy.ClusterWide {
		t.Errorf("kube proxy: %+v", proxy)
	}

	args, _ := s.Field("k8s_apiserver_extra_args")
	if args.Kind != KindObject {
		t.Errorf("extra args: %+v", args)
	}
}

// The names the screens of this TUI write today (fileio.ownedVarKeys). Which of
// them the installer beside it accepts is now a question the schema answers,
// rather than a list kept here.
func TestWhichOfTheScreensVariablesThisInstallerHas(t *testing.T) {
	screens := []string{
		"ki_var_root_path", "containerd_root_path", "docker_root_path",
		"internal_network_subnets", "internal_network_extra_zone", "internal_network_extra_zone_a_records",
		"ki_cp_ha_mode", "ki_cp_ha_mode_vip", "ki_cp_dns_dnssec_validation",
		"ki_cp_dns_server_upstream_servers", "ki_cp_ntp_server_upstream_servers",
		"k8s_certificate_validity_period", "k8s_load_balancers", "k8s_gateways", "k8s_default_ingress_class",
		"aipub_ingress_zone", "aipub_ha_mode", "aipub_ha_mode_storage_class", "aipub_cp_nodes",
		"aipub_harbor_ingress_subdomain", "aipub_harbor_replica_count",
		"aipub_harbor_registry_storage_size", "aipub_harbor_postgresql_storage_size",
		"aipub_harbor_redis_storage_size", "aipub_harbor_trivy_storage_size",
		"ki_cert_mode",
	}
	s := load(t)
	unknown := s.Unknown(screens)
	if len(unknown) != 15 {
		t.Errorf("%d of the screens' variables are unknown to main, want 15: %v", len(unknown), unknown)
	}
	// And the other way: what the installer has that no screen offers.
	onScreen := map[string]bool{}
	for _, n := range screens {
		onScreen[n] = true
	}
	missing := 0
	for _, f := range s.Fields {
		if !onScreen[f.Name] {
			missing++
		}
	}
	if missing != 52 {
		t.Errorf("%d variables of main have no screen, want 52", missing)
	}
	if !s.Has("ansible_user") || !s.Has("ansible_port") {
		t.Error("ansible_ names pass the installer's check and should pass here")
	}
}

func TestNotASchema(t *testing.T) {
	if _, err := Parse([]byte(`{"properties": {}}`)); err == nil {
		t.Error("a file with no x-ki-order should be refused")
	}
	if _, err := Parse([]byte(`{"x-ki-order": ["a"], "properties": {"b": {}}}`)); err == nil {
		t.Error("an order naming a missing property should be refused")
	}
}
