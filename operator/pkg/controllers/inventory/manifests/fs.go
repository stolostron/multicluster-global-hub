package manifests

import "embed"

//go:embed inventory-api
var InventoryManifestFiles embed.FS

// The spicedb manifest is from bundle.yaml
// https://github.com/authzed/spicedb-operator/releases/download/v1.18.0/bundle.yaml

//go:embed spicedb-operator
var SpiceDBOperatorManifestFiles embed.FS

//go:embed relations-api
var RelationsAPIFiles embed.FS
