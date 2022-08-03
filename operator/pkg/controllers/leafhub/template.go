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

package leafhub

import (
	"embed"
	"io/fs"
	"strings"
	"text/template"
)

// parseNonHypershiftTemplates parses nonhypershift templates from given FS
func parseNonHypershiftTemplates(manifestFS embed.FS) (*template.Template, error) {
	tpl := template.New("")
	err := fs.WalkDir(manifestFS, ".", func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (strings.HasSuffix(file, "subscription") || strings.HasSuffix(file, "mch") || strings.HasSuffix(file, "agent")) {
			manifests, err := readFilesInDir(manifestFS, file)
			if err != nil {
				return err
			}

			t := tpl.New(file)
			_, err = t.Parse(manifests)
			if err != nil {
				return err
			}
		}

		return nil
	})

	return tpl, err
}

// parseHubHypershiftTemplates parses hypershift templates from given FS
func parseHubHypershiftTemplates(manifestFS embed.FS, acmSnapshot, mceSnapshot, acmDefaultImageRegistry, mceDefaultImageRegistry string, acmImages, mceImages map[string]string) (*template.Template, error) {
	tf := template.FuncMap{
		"getACMImage": func(imageKey string) string {
			if acmSnapshot != "" {
				return acmDefaultImageRegistry + "/" + imageKey + ":" + acmSnapshot
			} else {
				return acmImages[imageKey]
			}
		},
		"getMCEImage": func(imageKey string) string {
			if mceSnapshot != "" {
				return mceDefaultImageRegistry + "/" + imageKey + ":" + mceSnapshot
			} else {
				return mceImages[imageKey]
			}
		},
	}

	tpl := template.New("")
	err := fs.WalkDir(manifestFS, ".", func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (strings.HasSuffix(file, "hosted") || strings.HasSuffix(file, "hosting")) {
			manifests, err := readFilesInDir(manifestFS, file)
			if err != nil {
				return err
			}

			t := tpl.New(file).Funcs(tf)
			_, err = t.Parse(manifests)
			if err != nil {
				return err
			}
		}

		return nil
	})

	return tpl, err
}

// parseAgentHypershiftTemplates parses hypershift templates from given FS
func parseAgentHypershiftTemplates(manifestFS embed.FS) (*template.Template, error) {
	tpl := template.New("")
	err := fs.WalkDir(manifestFS, ".", func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (strings.HasSuffix(file, "hosted") || strings.HasSuffix(file, "hosting")) {
			manifests, err := readFilesInDir(manifestFS, file)
			if err != nil {
				return err
			}

			t := tpl.New(file)
			_, err = t.Parse(manifests)
			if err != nil {
				return err
			}
		}

		return nil
	})

	return tpl, err
}

// readFilesInDir reads all files within the given directory
func readFilesInDir(manifestFS embed.FS, dir string) (string, error) {
	var res string
	err := fs.WalkDir(manifestFS, dir, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			b, err := manifestFS.ReadFile(file)
			if err != nil {
				return err
			}
			res += string(b) + "\n---\n"
		}
		return nil
	})

	return res, err
}
