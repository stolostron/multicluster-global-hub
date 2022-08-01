/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

// type PackageManifestConfig struct {
// 	ACMDefaultChannel string
// 	ACMCurrentCSV     string
// 	ACMImages         map[string]string
// 	MCEDefaultChannel string
// 	MCECurrentCSV     string
// 	MCEImages         map[string]string
// }

// var packageManifestConfig = &PackageManifestConfig{}

// func GetPackageManifestConfig() *PackageManifestConfig {
// 	return packageManifestConfig
// }

// func SetPackageManifestConfig(pm *PackageManifestConfig) {
// 	packageManifestConfig = pm
// }

// func SetPackageManifestACMConfig(ACMDefaultChannel, ACMCurrentCSV string, ACMImages map[string]string) {
// 	packageManifestConfig.ACMDefaultChannel = ACMDefaultChannel
// 	packageManifestConfig.ACMCurrentCSV = ACMCurrentCSV
// 	packageManifestConfig.ACMImages = ACMImages
// }

// func SetPackageManifestMCEConfig(MCEDefaultChannel, MCECurrentCSV string, MCEImages map[string]string) {
// 	packageManifestConfig.MCEDefaultChannel = MCEDefaultChannel
// 	packageManifestConfig.MCECurrentCSV = MCECurrentCSV
// 	packageManifestConfig.MCEImages = MCEImages
// }

// func EnsurePackageManifest(new *PackageManifestConfig) bool {
// 	existing := GetPackageManifestConfig()
// 	if existing.ACMCurrentCSV != new.ACMCurrentCSV ||
// 		existing.ACMDefaultChannel != new.ACMDefaultChannel {
// 		return true
// 	}
// 	return false
// }
