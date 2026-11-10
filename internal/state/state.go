package state

type AppState struct {
	// S1: Nodes (inventory.yml → all.hosts)
	Nodes []NodeConfig

	// S2: Groups (inventory.yml → ki_cp_node, k8s_node)
	// ControlNodeIH is control_node.hosts.localhost.control_node_ih: which ki_cp node
	// the control node is. k8s-installer stops without it (check-control-node.yml).
	ControlNodeIH  string
	KiCpNodes      []string
	K8sNodes       []string
	K8sCpNodes     []string
	NvidiaGPUNodes []string

	// S3: Network (vars.yml)
	InternalNetworkSubnets   []string
	KiCpHaMode               bool
	KiCpHaModeVIP            string
	KiCpDnsDnssecValidation  bool
	KiCpDnsUpstreamServers   []string
	KiCpNtpUpstreamServers   []string
	InternalDNSServerEnabled bool

	// S4: Kubernetes (vars.yml)
	K8sCertificateValidityPeriod string
	K8sIngressMode               int // 1=별도IP, 2=공유IP
	K8sIngressLBName             string
	K8sIngressVIP                string
	K8sGatewayLBName             string // mode 1 only
	K8sGatewayVIP                string // mode 1 only
	K8sIngressHostNetworkPort    int

	// S5: AIPub (vars.yml)
	AipubEnabled                bool
	AipubIngressZone            string
	AipubHaMode                 bool
	AipubHaModeStorageClass     string
	AipubCpNodes                []string
	HarborIngressSubdomain      string
	HarborReplicaCount          int
	HarborRegistryStorageSize   string
	HarborPostgresqlStorageSize string
	HarborRedisStorageSize      string
	HarborTrivyStorageSize      string

	// S6: Certificate mode (vars.yml)
	KiCertMode string

	// Schema mode: the installer beside the TUI says which variables vars.yml has
	// (ansible/schema/vars.schema.json). Screens 3-6 above are not used then.
	SchemaMode bool
	VarsOrder  []string               // the variables, in the order of the schema
	VarsValues map[string]interface{} // value of each: from vars.yml, else its shipped default
	VarsEdited map[string]bool        // the ones changed here, which a save writes
}

func DefaultState() *AppState {
	return &AppState{
		KiCpHaMode:                   true,
		KiCpDnsDnssecValidation:      false,
		KiCpDnsUpstreamServers:       []string{"8.8.8.8", "8.8.4.4"},
		KiCpNtpUpstreamServers:       []string{"time1.google.com", "time2.google.com"},
		InternalNetworkSubnets:       []string{"192.168.0.0/24"},
		K8sCertificateValidityPeriod: "26280h",
		K8sIngressMode:               2,
		K8sIngressLBName:             "lb1",
		K8sGatewayLBName:             "lb2",
		K8sIngressHostNetworkPort:    443,
		AipubEnabled:                 false,
		AipubIngressZone:             "example.com",
		AipubHaMode:                  true,
		HarborIngressSubdomain:       "aipub-harbor",
		HarborReplicaCount:           3,
		HarborRegistryStorageSize:    "512Gi",
		HarborPostgresqlStorageSize:  "128Gi",
		HarborRedisStorageSize:       "32Gi",
		HarborTrivyStorageSize:       "5Gi",
		KiCertMode:                   "self_signed",
	}
}

func (s *AppState) NodeNames() []string {
	names := make([]string, len(s.Nodes))
	for i, n := range s.Nodes {
		names[i] = n.Name
	}
	return names
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func (s *AppState) IsKiCpNode(name string) bool      { return contains(s.KiCpNodes, name) }
func (s *AppState) IsK8sNode(name string) bool       { return contains(s.K8sNodes, name) }
func (s *AppState) IsK8sCpNode(name string) bool     { return contains(s.K8sCpNodes, name) }
func (s *AppState) IsNvidiaGPUNode(name string) bool { return contains(s.NvidiaGPUNodes, name) }
func (s *AppState) IsAipubCpNode(name string) bool   { return contains(s.AipubCpNodes, name) }
func (s *AppState) IsControlNode(name string) bool   { return name != "" && s.ControlNodeIH == name }
