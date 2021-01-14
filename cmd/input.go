package cmd

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/builtinsupport"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/helpers"
)

func isInterrupt(err error) bool {
	return err != nil && err.Error() == "interrupt"
}

func confirm(message string) bool {
	confirm := false
	prompt := &survey.Confirm{
		Message: message,
	}
	err := survey.AskOne(prompt, &confirm)
	checkErr(err, nil)

	return confirm
}

func selectOrganization(client *api.Client, slug string) (*api.Organization, error) {
	orgs, err := client.GetOrganizations()
	if err != nil {
		return nil, err
	}

	if slug != "" {
		for _, org := range orgs {
			if org.Slug == slug {
				return &org, nil
			}
		}

		return nil, fmt.Errorf(`organization "%s" not found`, slug)
	}

	if len(orgs) == 1 && orgs[0].Type == "PERSONAL" {
		fmt.Printf("Automatically selected %s organization: %s\n", strings.ToLower(orgs[0].Type), orgs[0].Name)
		return &orgs[0], nil
	}

	sort.Slice(orgs[:], func(i, j int) bool { return orgs[i].Type < orgs[j].Type })

	options := []string{}

	for _, org := range orgs {
		options = append(options, fmt.Sprintf("%s (%s)", org.Name, org.Slug))
	}

	selectedOrg := 0
	prompt := &survey.Select{
		Message:  "Select organization:",
		Options:  options,
		PageSize: 15,
	}
	if err := survey.AskOne(prompt, &selectedOrg); err != nil {
		return nil, err
	}

	return &orgs[selectedOrg], nil
}

// func selectZone(client *api.Client, orgslug string, slug string) (string, error) {
// 	return "", nil
// 	// zones, err := client.GetDNSZones(orgslug)
// 	// if err != nil {
// 	// 	return "", err
// 	// }

// 	// sort.Slice(zones[:], func(i, j int) bool { return (*zones[i]).Domain < (*zones[j]).Domain })

// 	// options := []string{}

// 	// for _, zone := range zones {
// 	// 	options = append(options, fmt.Sprintf("%s", zone.Domain))
// 	// }

// 	// selectedOrg := 0
// 	// prompt := &survey.Select{
// 	// 	Message:  "Select zone:",
// 	// 	Options:  options,
// 	// 	PageSize: 15,
// 	// }
// 	// if err := survey.AskOne(prompt, &selectedOrg); err != nil {
// 	// 	return "", err
// 	// }

// 	// return (*zones[selectedOrg]).Domain, nil
// }

type suggestedBuilder struct {
	Vendor             string
	Image              string
	DefaultDescription string
}

var suggestedBuilders = []suggestedBuilder{
	{
		Vendor:             "Google",
		Image:              "gcr.io/buildpacks/builder",
		DefaultDescription: "GCP Builder for all runtimes",
	},
	{
		Vendor:             "Heroku",
		Image:              "heroku/buildpacks:18",
		DefaultDescription: "heroku-18 base image with buildpacks for Ruby, Java, Node.js, Python, Golang, & PHP",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "gcr.io/paketo-buildpacks/builder:base",
		DefaultDescription: "Small base image with buildpacks for Java, Node.js, Golang, & .NET Core",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "gcr.io/paketo-buildpacks/builder:full-cf",
		DefaultDescription: "Larger base image with buildpacks for Java, Node.js, Golang, .NET Core, & PHP",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "gcr.io/paketo-buildpacks/builder:tiny",
		DefaultDescription: "Tiny base image (bionic build image, distroless run image) with buildpacks for Golang",
	},
	{
		Vendor:             "Fly",
		Image:              "flyio/builder",
		DefaultDescription: "Fly's own Buildpack - currently supporting Deno",
	},
}

var builtins []builtinsupport.Builtin

func selectBuildtype(commandContext *cmdctx.CmdContext) (string, bool, error) {

	dockerfileExists := helpers.FileExists(path.Join(commandContext.WorkingDir, "Dockerfile"))

	builders := []string{}

	// None/Dockerfile first - always entry 0
	if dockerfileExists {
		builders = append(builders, fmt.Sprintf("%s\n    (%s)", "Dockerfile", "Do not set a builder and use the existing Dockerfile"))
	} else {
		builders = append(builders, fmt.Sprintf("%s\n    (%s)", "None", "Do not set a builder"))
	}

	builders = append(builders, fmt.Sprintf("%s\n    (%s)", "Image", "Use a public Docker image"))

	builtins = builtinsupport.GetBuiltins(commandContext)

	sort.Slice(builtins, func(i, j int) bool { return builtins[i].Name < builtins[j].Name })

	builtinsStart := len(builders)
	for _, b := range builtins {
		builders = append(builders, fmt.Sprintf("%s\n    %s", b.Name, helpers.WrapString(b.Description, 60, 4)))
	}

	buildersStart := len(builders)
	for _, b := range suggestedBuilders {
		builders = append(builders, fmt.Sprintf("%s\n    %s", b.Image, helpers.WrapString(b.DefaultDescription, 70, 4)))
	}

	selectedBuilder := 0

	prompt := &survey.Select{
		Message:  "Select builder:",
		Options:  builders,
		PageSize: 8,
	}

	if err := survey.AskOne(prompt, &selectedBuilder); err != nil {
		return "", false, err
	}

	// Selected a Dockerfile or the absence of a builder
	if selectedBuilder == 0 {
		if dockerfileExists {
			return "Dockerfile", false, nil
		}
		return "None", false, nil
	}

	// Selected an image
	if selectedBuilder == 1 {
		return "Image", false, nil
	}

	if selectedBuilder >= builtinsStart && selectedBuilder < buildersStart {
		// Selected a built in
		return builtins[selectedBuilder-builtinsStart].Name, true, nil
	}

	// All that is lef is builders by name
	return suggestedBuilders[selectedBuilder-buildersStart].Image, false, nil
}

func selectBuiltin(commandContext *cmdctx.CmdContext) (string, error) {
	availablebuiltins := []string{}

	builtins := builtinsupport.GetBuiltins(commandContext)

	sort.Slice(builtins, func(i, j int) bool { return builtins[i].Name < builtins[j].Name })

	for _, b := range builtins {
		availablebuiltins = append(availablebuiltins, fmt.Sprintf("%s\n    %s", b.Name, helpers.WrapString(b.Description, 60, 4)))
	}

	selectedBuiltin := 0

	prompt := &survey.Select{
		Message:  "Select builtin:",
		Options:  availablebuiltins,
		PageSize: 8,
	}
	if err := survey.AskOne(prompt, &selectedBuiltin); err != nil {
		return "", err
	}

	return builtins[selectedBuiltin].Name, nil

}

func selectImage(commandContext *cmdctx.CmdContext) (string, error) {
	prompt := &survey.Input{Message: "Select Image:", Default: "flyio/hellofly:latest", Help: `The name and tag for the image you want to use.`}

	sSelectedImage := ""
	if err := survey.AskOne(prompt, &sSelectedImage /* survey.WithValidator(isIntPort) */); err != nil {
		return sSelectedImage, err
	}

	return sSelectedImage, nil
}

func selectPort(commandContext *cmdctx.CmdContext, defport int) (int, error) {
	sDefport := strconv.Itoa(defport)
	prompt := &survey.Input{Message: "Select Internal Port:", Default: sDefport, Help: `The internal port is the port your application uses. External traffic will be directed to this port.
If incorrectly set, health checks may fail and your application deployment will fail.`}

	sSelectedPort := ""
	if err := survey.AskOne(prompt, &sSelectedPort, survey.WithValidator(isIntPort)); err != nil {
		return -1, err
	}
	selectedPort, err := strconv.Atoi(sSelectedPort)

	if err != nil {
		return -1, errors.New("number did not parse after verification")
	}

	return selectedPort, nil
}

func isIntPort(val interface{}) error {
	str, ok := val.(string)

	if ok {
		val, err := strconv.Atoi(str)

		if err != nil {
			return errors.New("has to be an integer")
		}

		if val < 1 || val > 65536 {
			return errors.New("port must be between 1 and 65536")
		}

		return nil
	}

	return errors.New("couldn't convert to string")
}
