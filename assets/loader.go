package assets

import "fmt"

// DiscoveryDocForDomain returns the embedded Discovery doc bytes for a dcx domain.
func DiscoveryDocForDomain(domain string) ([]byte, error) {
	switch domain {
	case "bigquery":
		return BigQueryDiscovery, nil
	case "spanner":
		return SpannerDiscovery, nil
	case "alloydb":
		return AlloyDBDiscovery, nil
	case "cloudsql":
		return CloudSQLDiscovery, nil
	case "looker":
		return LookerDiscovery, nil
	default:
		return nil, fmt.Errorf("no Discovery doc for domain %q", domain)
	}
}
