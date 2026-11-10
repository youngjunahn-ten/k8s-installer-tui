package schema

import (
	"reflect"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

func TestParseInput(t *testing.T) {
	s := load(t)
	cases := []struct {
		name, text string
		want       interface{}
		refused    bool
	}{
		{"ki_cp_ha_mode_vip", "192.168.0.250", "192.168.0.250", false},
		{"ki_cp_ha_mode_vip", "not-an-address", nil, true},
		{"ki_cp_ha_mode_vip", "", nil, false}, // nullable: empty is null
		{"k8s_pod_subnet", "10.244.0.0/16", "10.244.0.0/16", false},
		{"k8s_pod_subnet", "10.244.0.0", nil, true},
		{"k8s_pod_subnet", "", nil, true}, // required and not nullable
		{"internal_network_subnets", "192.168.0.0/24, 10.0.0.0/8", []interface{}{"192.168.0.0/24", "10.0.0.0/8"}, false},
		{"internal_network_subnets", "192.168.0.0/24, nope", nil, true},
		{"ki_cp_ntp_server_upstream_servers", "time1.google.com, 10.0.0.1", []interface{}{"time1.google.com", "10.0.0.1"}, false},
		{"kubelet_max_pods", "250", 250, false},
		{"kubelet_max_pods", "many", nil, true},
		{"k8s_minor_version", "1.36", "1.36", false},
		{"k8s_minor_version", "v1.36", nil, true}, // the pattern of the validator
		{"k8s_certificate_validity_period", "26280h", "26280h", false},
		{"k8s_certificate_validity_period", "3y", nil, true},
		{"ki_cp_ha_mode_vip_carrier", "aws-eni", "aws-eni", false},
		{"ki_cp_ha_mode_vip_carrier", "nlb", nil, true},
	}
	for _, c := range cases {
		f, ok := s.Field(c.name)
		if !ok {
			t.Fatalf("no field %s", c.name)
		}
		got, err := f.ParseInput(c.text)
		if c.refused != (err != nil) {
			t.Errorf("%s = %q: refused %v, want %v (%v)", c.name, c.text, err != nil, c.refused, err)
			continue
		}
		if !c.refused && !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s = %q: got %#v, want %#v", c.name, c.text, got, c.want)
		}
	}
}

// Every value the installer ships can be shown and typed back unchanged, so
// opening a variable and pressing Enter never changes it.
func TestShowReadsBack(t *testing.T) {
	s := load(t)
	for _, f := range s.Fields {
		if f.Kind == KindObject || f.Commented {
			continue
		}
		text := f.Show(f.Default, true)
		got, err := f.ParseInput(text)
		if err != nil {
			t.Errorf("%s: the shipped value %q is refused: %v", f.Name, text, err)
			continue
		}
		if !reflect.DeepEqual(got, f.Default) {
			t.Errorf("%s: %#v shown as %q reads back as %#v", f.Name, f.Default, text, got)
		}
	}
}

func TestFill(t *testing.T) {
	s := load(t)
	st := state.DefaultState()
	s.Fill(st, map[string]interface{}{"kubelet_max_pods": 200})

	if !st.SchemaMode || len(st.VarsOrder) != 63 {
		t.Fatalf("mode %v, %d variables", st.SchemaMode, len(st.VarsOrder))
	}
	if st.VarsValues["kubelet_max_pods"] != 200 {
		t.Error("the value of the file should win over the default")
	}
	if st.VarsValues["ki_etcd_backup_retention_count"] != 5 {
		t.Errorf("default %#v, want int 5", st.VarsValues["ki_etcd_backup_retention_count"])
	}
	if _, set := st.VarsValues["internal_network_extra_zone"]; set {
		t.Error("a variable shipped commented out should start unset")
	}
}
