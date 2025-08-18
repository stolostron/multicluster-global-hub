package renderer_test

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gonvenience/ytbx"
	"github.com/homeport/dyff/pkg/dyff"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
)

//go:embed testdata
var fs embed.FS

type NginxConfigValues struct {
	Labels             map[string]string            `json:"labels,omitempty"`
	Annotations        map[string]string            `json:"annotations,omitempty"`
	Autoscaling        Autoscaling                  `json:"autoscaling,omitempty"`
	Replicas           int                          `json:"replicas,omitempty"`
	PodLabels          map[string]string            `json:"podLabels,omitempty"`
	PodAnnotations     map[string]string            `json:"podAnnotations,omitempty"`
	PodSecurityContext map[string]interface{}       `json:"podSecurityContext,omitempty"`
	ServiceAccount     ServiceAccount               `json:"serviceAccount,omitempty"`
	Image              Image                        `json:"image,omitempty"`
	Resources          map[string]map[string]string `json:"resources,omitempty"`
	NodeSelector       map[string]string            `json:"nodeSelector,omitempty"`
	Tolerations        Tolerations                  `json:"tolerations,omitempty"`
	ServicePort        int                          `json:"servicePort,omitempty"`
	Ingress            Ingress                      `json:"ingress,omitempty"`
}

type Autoscaling struct {
	Enabled                           bool `json:"enabled,omitempty"`
	MinReplicas                       int  `json:"minReplicas,omitempty"`
	MaxReplicas                       int  `json:"maxReplicas,omitempty"`
	TargetCPUUtilizationPercentage    int  `json:"targetCPUUtilizationPercentage,omitempty"`
	TargetMemoryUtilizationPercentage int  `json:"targetMemoryUtilizationPercentage,omitempty"`
}

type ServiceAccount struct {
	Create bool   `json:"create,omitempty"`
	Name   string `json:"name,omitempty"`
}

type Image struct {
	PullSecrets []string `json:"pullSecrets,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	Tag         string   `json:"tag,omitempty"`
	PullPolicy  string   `json:"pullPolicy,omitempty"`
}

type Tolerations []struct {
	Key               string `json:"key,omitempty"`
	Operator          string `json:"operator,omitempty"`
	Value             string `json:"value,omitempty"`
	Effect            string `json:"effect,omitempty"`
	TolerationSeconds int    `json:"tolerationSeconds,omitempty"`
}

type Ingress struct {
	Enabled     bool              `json:"enabled,omitempty"`
	ClassName   string            `json:"className,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Hosts       []Host            `json:"hosts,omitempty"`
	TLS         []TLSConfig       `json:"TLS,omitempty"`
}

type Host struct {
	Host  string `json:"host,omitempty"`
	Paths []Path `json:"paths,omitempty"`
}

type Path struct {
	Path     string `json:"path,omitempty"`
	PathType string `json:"pathType,omitempty"`
}

type TLSConfig struct {
	SecretName string   `json:"secretName,omitempty"`
	Hosts      []string `json:"hosts,omitempty"`
}

var _ = Describe("Render", func() {
	Context("Render manifests with different profile", func() {
		var (
			render  = renderer.NewHoHRenderer(fs)
			profile = "default"
		)
		It("Should render nginx objects with default profile that are equal to golden manifests", func() {
			profile = "default"
			nginxObjects, err := render.Render("testdata/app", profile, func(profile string) (interface{}, error) {
				inputvalues, err := fs.ReadFile(fmt.Sprintf("testdata/input/%s.yaml", profile))
				if err != nil {
					return struct{}{}, err
				}

				nginxConfig := &NginxConfigValues{}
				if err = yaml.Unmarshal([]byte(inputvalues), nginxConfig); err != nil {
					return struct{}{}, err
				}

				return nginxConfig, nil
			})
			Expect(err).To(BeNil())

			tempFile, err := os.CreateTemp("", fmt.Sprintf("%s-", profile))
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tempFile.Name()) }()

			for _, unsObj := range nginxObjects {
				unsObjYaml, err := yaml.Marshal(unsObj)
				Expect(err).To(BeNil())
				_, err = tempFile.Write(unsObjYaml)
				Expect(err).To(BeNil())
				_, _ = tempFile.WriteString("\n---\n") // add yaml separator
			}

			goldenFilePath := filepath.Join("testdata/output", fmt.Sprintf("%s.golden.yaml", profile))
			goldenFileInput, err := ytbx.LoadFile(goldenFilePath)
			Expect(err).To(BeNil())
			tempFileInput, err := ytbx.LoadFile(tempFile.Name())
			Expect(err).To(BeNil())

			results, err := dyff.CompareInputFiles(goldenFileInput, tempFileInput, dyff.IgnoreOrderChanges(true))
			Expect(err).To(BeNil())
			Expect(results).NotTo(BeNil())
			Expect(results.Diffs).To(BeNil())
		})

		It("Should render nginx objects with ha profile that are equal to golden manifests", func() {
			profile = "ha"
			nginxObjects, err := render.Render("testdata/app", profile, func(profile string) (interface{}, error) {
				inputvalues, err := fs.ReadFile(fmt.Sprintf("testdata/input/%s.yaml", profile))
				if err != nil {
					return struct{}{}, err
				}

				nginxConfig := &NginxConfigValues{}
				if err = yaml.Unmarshal([]byte(inputvalues), nginxConfig); err != nil {
					return struct{}{}, err
				}

				return nginxConfig, nil
			})
			Expect(err).To(BeNil())

			tempFile, err := os.CreateTemp("", fmt.Sprintf("%s-", profile))
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tempFile.Name()) }()

			for _, unsObj := range nginxObjects {
				unsObjYaml, err := yaml.Marshal(unsObj)
				Expect(err).To(BeNil())
				_, err = tempFile.Write(unsObjYaml)
				Expect(err).To(BeNil())
				_, _ = tempFile.WriteString("\n---\n") // add yaml separator
			}

			goldenFilePath := filepath.Join("testdata/output", fmt.Sprintf("%s.golden.yaml", profile))
			goldenFileInput, err := ytbx.LoadFile(goldenFilePath)
			Expect(err).To(BeNil())
			tempFileInput, err := ytbx.LoadFile(tempFile.Name())
			Expect(err).To(BeNil())

			results, err := dyff.CompareInputFiles(goldenFileInput, tempFileInput, dyff.IgnoreOrderChanges(true))
			Expect(err).To(BeNil())
			Expect(results).NotTo(BeNil())
			Expect(results.Diffs).To(BeNil())
		})

		It("Should render nginx objects with external profile that are equal to golden manifests", func() {
			profile = "external"
			nginxObjects, err := render.Render("testdata/app", profile, func(profile string) (interface{}, error) {
				inputvalues, err := fs.ReadFile(fmt.Sprintf("testdata/input/%s.yaml", profile))
				if err != nil {
					return struct{}{}, err
				}

				nginxConfig := &NginxConfigValues{}
				if err = yaml.Unmarshal([]byte(inputvalues), nginxConfig); err != nil {
					return struct{}{}, err
				}

				return nginxConfig, nil
			})
			Expect(err).To(BeNil())

			tempFile, err := os.CreateTemp("", fmt.Sprintf("%s-", profile))
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tempFile.Name()) }()

			for _, unsObj := range nginxObjects {
				unsObjYaml, err := yaml.Marshal(unsObj)
				Expect(err).To(BeNil())
				_, err = tempFile.Write(unsObjYaml)
				Expect(err).To(BeNil())
				_, _ = tempFile.WriteString("\n---\n") // add yaml separator
			}

			goldenFilePath := filepath.Join("testdata/output", fmt.Sprintf("%s.golden.yaml", profile))
			goldenFileInput, err := ytbx.LoadFile(goldenFilePath)
			Expect(err).To(BeNil())
			tempFileInput, err := ytbx.LoadFile(tempFile.Name())
			Expect(err).To(BeNil())

			results, err := dyff.CompareInputFiles(goldenFileInput, tempFileInput, dyff.IgnoreOrderChanges(true))
			Expect(err).To(BeNil())
			Expect(results).NotTo(BeNil())
			Expect(results.Diffs).To(BeNil())
		})

		It("Should render nginx objects with all profile that are equal to golden manifests", func() {
			profile = "all"
			nginxObjects, err := render.Render("testdata/app", profile, func(profile string) (interface{}, error) {
				inputvalues, err := fs.ReadFile(fmt.Sprintf("testdata/input/%s.yaml", profile))
				if err != nil {
					return struct{}{}, err
				}

				nginxConfig := &NginxConfigValues{}
				if err = yaml.Unmarshal([]byte(inputvalues), nginxConfig); err != nil {
					return struct{}{}, err
				}

				return nginxConfig, nil
			})
			Expect(err).To(BeNil())

			tempFile, err := os.CreateTemp("", fmt.Sprintf("%s-", profile))
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tempFile.Name()) }()

			for _, unsObj := range nginxObjects {
				unsObjYaml, err := yaml.Marshal(unsObj)
				Expect(err).To(BeNil())
				_, err = tempFile.Write(unsObjYaml)
				Expect(err).To(BeNil())
				_, _ = tempFile.WriteString("\n---\n") // add yaml separator
			}

			goldenFilePath := filepath.Join("testdata/output", fmt.Sprintf("%s.golden.yaml", profile))
			goldenFileInput, err := ytbx.LoadFile(goldenFilePath)
			Expect(err).To(BeNil())
			tempFileInput, err := ytbx.LoadFile(tempFile.Name())
			Expect(err).To(BeNil())

			results, err := dyff.CompareInputFiles(goldenFileInput, tempFileInput, dyff.IgnoreOrderChanges(true))
			Expect(err).To(BeNil())
			Expect(results).NotTo(BeNil())
			Expect(results.Diffs).To(BeNil())
		})

		It("Should render nginx objects with all profile and filter that are equal to golden manifests", func() {
			profile = "all"
			filter := "nginx"
			nginxObjects, err := render.RenderWithFilter("testdata/app", profile,
				filter, func(profile string) (interface{}, error) {
					inputvalues, err := fs.ReadFile(fmt.Sprintf(
						"testdata/input/%s-%s.yaml", profile, filter))
					if err != nil {
						return struct{}{}, err
					}

					nginxConfig := &NginxConfigValues{}
					if err = yaml.Unmarshal([]byte(inputvalues), nginxConfig); err != nil {
						return struct{}{}, err
					}

					return nginxConfig, nil
				})
			Expect(err).To(BeNil())

			tempFile, err := os.CreateTemp("", fmt.Sprintf("%s-%s-", profile, filter))
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tempFile.Name()) }()

			for _, unsObj := range nginxObjects {
				unsObjYaml, err := yaml.Marshal(unsObj)
				Expect(err).To(BeNil())
				_, err = tempFile.Write(unsObjYaml)
				Expect(err).To(BeNil())
				_, _ = tempFile.WriteString("\n---\n") // add yaml separator
			}

			goldenFilePath := filepath.Join("testdata/output",
				fmt.Sprintf("%s-%s.golden.yaml", profile, filter))
			goldenFileInput, err := ytbx.LoadFile(goldenFilePath)
			Expect(err).To(BeNil())
			tempFileInput, err := ytbx.LoadFile(tempFile.Name())
			Expect(err).To(BeNil())

			results, err := dyff.CompareInputFiles(goldenFileInput, tempFileInput, dyff.IgnoreOrderChanges(true))
			Expect(err).To(BeNil())
			Expect(results).NotTo(BeNil())
			Expect(results.Diffs).To(BeNil())
		})
	})
})
