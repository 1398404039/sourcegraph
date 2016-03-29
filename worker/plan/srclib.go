package plan

import (
	"fmt"
	"net/url"
	"strings"

	droneyaml "github.com/drone/drone-exec/yaml"
	"github.com/drone/drone/yaml/matrix"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/inventory"
)

// configureSrclib modifies the Drone config to run srclib analysis
// during the CI build.
func configureSrclib(inv *inventory.Inventory, config *droneyaml.Config, axes []matrix.Axis, srclibImportURL, srclibCoverageURL *url.URL) error {
	var srclibExplicitlyConfigured bool
	for _, step := range config.Build {
		// Rough heuristic for now: does the Docker image name contain
		// "srclib".
		// (alexsaveliev) excluding srclib-java from heuristic
		// because it's used both by build/test and srclib steps
		if strings.Contains(step.Container.Image, "srclib") &&
			!strings.Contains(step.Container.Image, "srclib-java") {
			srclibExplicitlyConfigured = true
			break
		}
	}

	usingSrclib := srclibExplicitlyConfigured // track if we've found any srclib languages

	// Add the srclib build steps for all of the languages we
	// detect. But if we've explicitly configured srclib at all, then
	// don't do any automagic.
	if !srclibExplicitlyConfigured {
		for _, lang := range inv.Languages {
			b, ok := langSrclibConfigs[lang.Name]
			if ok {
				usingSrclib = true
			} else {
				b = buildLogMsg(fmt.Sprintf("Code Intelligence does not yet support %s", lang.Name), fmt.Sprintf("Sourcegraph Code Intelligence does not yet support %s (which was detected in this repository)", lang.Name))
			}

			if err := insertSrclibBuild(config, axes, b); err != nil {
				return err
			}
		}
	}

	if !usingSrclib {
		if err := insertSrclibBuild(config, axes, buildLogMsg("Code Intelligence did not find any supported programming languages", "no supported programming languages were auto-detected for Sourcegraph Code Intelligence")); err != nil {
			return err
		}
	}

	// Insert the srclib import build step, only if we found an actual
	// srclib analyzer already (or one was explicitly configured).
	if srclibImportURL != nil && usingSrclib {
		if err := insertSrclibBuild(config, axes, srclibImportStep(srclibImportURL)); err != nil {
			return err
		}
	}

	if srclibCoverageURL != nil && usingSrclib {
		if err := insertSrclibBuild(config, axes, srclibCoverageStep(srclibCoverageURL)); err != nil {
			return err
		}
	}

	return nil
}

var droneSrclibGoImage = "sourcegraph/srclib-go:latest"

// Note: If you push new Docker images for the srclib build steps, you
// MUST update the SHA256 digest, or else users will continue using
// the old Docker image. Also ensure you `docker push` the new Docker
// images, or else users' builds will fail because the required image
// is not available on the Docker Hub.
var langSrclibConfigs = map[string]droneyaml.BuildItem{
	"Go": droneyaml.BuildItem{
		Key: "Go (indexing)",
		Build: droneyaml.Build{
			Container: droneyaml.Container{
				Image: droneSrclibGoImage,
			},
			Commands:     srclibBuildCommands,
			AllowFailure: true,
		},
	},
	"JavaScript": droneyaml.BuildItem{
		Key: "JavaScript (indexing)",
		Build: droneyaml.Build{
			Container: droneyaml.Container{
				Image: "sourcegraph/srclib-javascript:latest",
			},
			Commands:     srclibBuildCommands,
			AllowFailure: true,
		},
	},
	"Java": droneyaml.BuildItem{
		Key: "Java (indexing)",
		Build: droneyaml.Build{
			Container: droneyaml.Container{
				Image: "sourcegraph/srclib-java:latest",
			},
			Commands:     srclibBuildCommands,
			AllowFailure: true,
		},
	},
	"PHP": droneyaml.BuildItem{
		Key: "PHP (indexing)",
		Build: droneyaml.Build{
			Container: droneyaml.Container{
				Image: "sourcegraph/srclib-basic:latest",
			},
			Commands:     srclibBuildCommands,
			AllowFailure: true,
		},
	},
	"Objective-C": droneyaml.BuildItem{
		Key: "Objective-C (indexing)",
		Build: droneyaml.Build{
			Container: droneyaml.Container{
				Image: "sourcegraph/srclib-basic:latest",
			},
			Commands:     srclibBuildCommands,
			AllowFailure: true,
		},
	},
	"Python": droneyaml.BuildItem{
		Key: "Python (indexing)",
		Build: droneyaml.Build{
			Container: droneyaml.Container{
				Image: "sourcegraph/srclib-python:latest",
			},
			Commands:     srclibBuildCommands,
			AllowFailure: true,
		},
	},
	"TypeScript": droneyaml.BuildItem{
		Key: "TypeScript (indexing)",
		Build: droneyaml.Build{
			Container: droneyaml.Container{
				Image: "sourcegraph/srclib-typescript:latest",
			},
			Commands:     srclibBuildCommands,
			AllowFailure: true,
		},
	},
	"C#": droneyaml.BuildItem{
		Key: "C# (indexing)",
		Build: droneyaml.Build{
			Container: droneyaml.Container{
				Image: "srclib/drone-srclib-csharp@sha256:28a3d40e363c94b61bed2d9f363602fe502953df1659d207e5c30e5e5df2ef80",
			},
			Commands:     srclibBuildCommands,
			AllowFailure: true,
		},
	},
}

var srclibBuildCommands = []string{"srclib config", "srclib make"}

// srclibImportStep returns a Drone build step that imports srclib
// data to the httpapi srclib import endpoint given by importURL
// (e.g., http://localhost:3080/.api/repos/my/repo/.srclib-import).
func srclibImportStep(importURL *url.URL) droneyaml.BuildItem {
	return droneyaml.BuildItem{
		Key: "srclib import",
		Build: droneyaml.Build{
			Container: droneyaml.Container{
				// The hash is the final line of docker push output
				Image: "sourcegraph/srclib-import@sha256:19d2918264ac24a928c06f3226aad3bf6babc8a8181dcf1cdc5a200c186006cd",
				Environment: droneyaml.MapEqualSlice([]string{
					"SOURCEGRAPH_IMPORT_URL=" + importURL.String(),
				}),
			},
			Commands: []string{
				"echo Importing to $SOURCEGRAPH_IMPORT_URL",
				`files=$(find .srclib-cache/ -type f | head -n 1); if [ -z "$files" ]; then echo No srclib data files found to import; exit 0; fi`,
				`cd .srclib-cache/* && /usr/bin/zip -q --no-dir-entries -r - . > /tmp/srclib-cache.zip`,
				`srclib-import $SOURCEGRAPH_IMPORT_URL /tmp/srclib-cache.zip`,
				"echo Done importing",
			},
		},
	}
}

func srclibCoverageStep(coverageURL *url.URL) droneyaml.BuildItem {
	return droneyaml.BuildItem{
		Key: "Graph metrics",
		Build: droneyaml.Build{
			Container: droneyaml.Container{
				Image: droneSrclibGoImage,
				Environment: droneyaml.MapEqualSlice([]string{
					"SOURCEGRAPH_COVERAGE_URL=" + coverageURL.String(),
				}),
			},
			Commands: []string{
				"echo Generating srclib coverage stats",
				"srclib coverage > /tmp/srclib-coverage.json",
				"cat /tmp/srclib-coverage.json",
				"echo Publishing srclib coverage stats",
				`cat /tmp/srclib-coverage.json | /usr/bin/curl \
				--silent --show-error \
				--netrc \
				--max-time 300 \
				--no-keepalive \
				--retry 3 \
				--retry-delay 2 \
				-XPUT \
				-H 'Content-Type: application/json' \
				-H 'Content-Transfer-Encoding: binary' \
				--data-binary @- \
				$SOURCEGRAPH_COVERAGE_URL`,
				"echo Done publishing",
			},
			AllowFailure: true, // This step failing is not critical to user operations, so do not block the build
		},
	}
}

// insertSrclibBuild inserts a build into the YAML. If there is a build
// matrix, the step will only execute for a single cell in the matrix.
func insertSrclibBuild(config *droneyaml.Config, axes []matrix.Axis, build droneyaml.BuildItem) error {
	if len(axes) < 1 {
		panic("must have at least 1 axis")
	}
	if len(axes) > 1 {
		build.Filter = droneyaml.Filter{Matrix: axes[0]}
	}

	{
		// If the build section is a single-build section, then we need to
		// convert it into a multi-build section and give the lone
		// existing build step a default name ("build").
		v, err := config.Build.MarshalYAML()
		if err != nil {
			return err
		}
		if v, ok := v.(droneyaml.Build); ok {
			// Is a single-section build.
			config.Build = droneyaml.Builds{{Key: "build", Build: v}}
		}
	}

	config.Build = append(config.Build, build)
	return nil
}
