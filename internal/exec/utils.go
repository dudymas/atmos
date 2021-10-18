package exec

import (
	"errors"
	"fmt"
	c "github.com/cloudposse/atmos/internal/config"
	g "github.com/cloudposse/atmos/internal/globals"
	s "github.com/cloudposse/atmos/internal/stack"
	u "github.com/cloudposse/atmos/internal/utils"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"strings"
)

var (
	commonFlags = []string{
		"--stack",
		"-s",
		"--dry-run",
		"--kubeconfig-path",
		g.TerraformDirFlag,
		g.HelmfileDirFlag,
		g.ConfigDirFlag,
		g.StackDirFlag,
		g.GlobalOptionsFlag,
		g.DeployRunInitFlag,
	}
)

// checkStackConfig checks stack schema and return component info
func checkStackConfig(
	stack string,
	stacksMap map[string]interface{},
	componentType string,
	component string,
) (map[string]interface{}, map[interface{}]interface{}, map[interface{}]interface{}, string, string, error) {

	var stackSection map[interface{}]interface{}
	var componentsSection map[string]interface{}
	var componentTypeSection map[string]interface{}
	var componentSection map[string]interface{}
	var componentVarsSection map[interface{}]interface{}
	var componentBackendSection map[interface{}]interface{}
	var baseComponentPath string
	var command string
	var ok bool

	if stackSection, ok = stacksMap[stack].(map[interface{}]interface{}); !ok {
		return nil, nil, nil, "", "", errors.New(fmt.Sprintf("Stack '%s' does not exist", stack))
	}
	if componentsSection, ok = stackSection["components"].(map[string]interface{}); !ok {
		return nil, nil, nil, "", "", errors.New(fmt.Sprintf("'components' section is missing in the stack '%s'", stack))
	}
	if componentTypeSection, ok = componentsSection[componentType].(map[string]interface{}); !ok {
		return nil, nil, nil, "", "", errors.New(fmt.Sprintf("'components/%s' section is missing in the stack '%s'", componentType, stack))
	}
	if componentSection, ok = componentTypeSection[component].(map[string]interface{}); !ok {
		return nil, nil, nil, "", "", errors.New(fmt.Sprintf("Invalid or missing configuration for the component '%s' in the stack '%s'", component, stack))
	}
	if componentVarsSection, ok = componentSection["vars"].(map[interface{}]interface{}); !ok {
		return nil, nil, nil, "", "", errors.New(fmt.Sprintf("Missing 'vars' section for the component '%s' in the stack '%s'", component, stack))
	}
	if componentBackendSection, ok = componentSection["backend"].(map[interface{}]interface{}); !ok {
		componentBackendSection = nil
	}
	if baseComponentPath, ok = componentSection["component"].(string); !ok {
		baseComponentPath = ""
	}
	if command, ok = componentSection["command"].(string); !ok {
		command = ""
	}

	return componentSection, componentVarsSection, componentBackendSection, baseComponentPath, command, nil
}

// findComponentConfig finds component config sections
func findComponentConfig(
	stack string,
	stacksMap map[string]interface{},
	componentType string,
	component string,
) (map[string]interface{}, map[interface{}]interface{}, map[interface{}]interface{}, error) {

	var stackSection map[interface{}]interface{}
	var componentsSection map[string]interface{}
	var componentTypeSection map[string]interface{}
	var componentSection map[string]interface{}
	var componentVarsSection map[interface{}]interface{}
	var componentBackendSection map[interface{}]interface{}
	var ok bool

	if stackSection, ok = stacksMap[stack].(map[interface{}]interface{}); !ok {
		return nil, nil, nil, errors.New(fmt.Sprintf("Stack '%s' does not exist", stack))
	}
	if componentsSection, ok = stackSection["components"].(map[string]interface{}); !ok {
		return nil, nil, nil, errors.New(fmt.Sprintf("'components' section is missing in the stack '%s'", stack))
	}
	if componentTypeSection, ok = componentsSection[componentType].(map[string]interface{}); !ok {
		return nil, nil, nil, errors.New(fmt.Sprintf("'components/%s' section is missing in the stack '%s'", componentType, stack))
	}
	if componentSection, ok = componentTypeSection[component].(map[string]interface{}); !ok {
		return nil, nil, nil, errors.New(fmt.Sprintf("Invalid or missing configuration for the component '%s' in the stack '%s'", component, stack))
	}
	if componentVarsSection, ok = componentSection["vars"].(map[interface{}]interface{}); !ok {
		return nil, nil, nil, errors.New(fmt.Sprintf("Missing 'vars' section for the component '%s' in the stack '%s'", component, stack))
	}
	if componentBackendSection, ok = componentSection["backend"].(map[interface{}]interface{}); !ok {
		componentBackendSection = nil
	}

	return componentSection, componentVarsSection, componentBackendSection, nil
}

// processConfigAndStacks processes CLI config and stacks
func processConfigAndStacks(componentType string, cmd *cobra.Command, args []string) (c.ConfigAndStacksInfo, error) {
	var configAndStacksInfo c.ConfigAndStacksInfo

	if len(args) < 1 {
		return configAndStacksInfo, errors.New("invalid number of arguments")
	}

	cmd.DisableFlagParsing = false

	err := cmd.ParseFlags(args)
	if err != nil {
		return configAndStacksInfo, err
	}
	flags := cmd.Flags()

	configAndStacksInfo.Stack, err = flags.GetString("stack")
	if err != nil {
		return configAndStacksInfo, err
	}

	argsAndFlagsInfo, err := processArgsAndFlags(args)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.AdditionalArgsAndFlags = argsAndFlagsInfo.AdditionalArgsAndFlags
	configAndStacksInfo.SubCommand = argsAndFlagsInfo.SubCommand
	configAndStacksInfo.ComponentFromArg = argsAndFlagsInfo.ComponentFromArg
	configAndStacksInfo.GlobalOptions = argsAndFlagsInfo.GlobalOptions
	configAndStacksInfo.TerraformDir = argsAndFlagsInfo.TerraformDir
	configAndStacksInfo.HelmfileDir = argsAndFlagsInfo.HelmfileDir
	configAndStacksInfo.StacksDir = argsAndFlagsInfo.StacksDir
	configAndStacksInfo.ConfigDir = argsAndFlagsInfo.ConfigDir
	configAndStacksInfo.DeployRunInit = argsAndFlagsInfo.DeployRunInit

	// Check if component was provided
	if len(configAndStacksInfo.ComponentFromArg) < 1 {
		return configAndStacksInfo, errors.New("'component' is required")
	}

	// Process and merge CLI configurations
	err = c.InitConfig(configAndStacksInfo)
	if err != nil {
		return configAndStacksInfo, err
	}

	// Process stack config file(s)
	_, stacksMap, err := s.ProcessYAMLConfigFiles(
		c.ProcessedConfig.StacksBaseAbsolutePath,
		c.ProcessedConfig.StackConfigFilesAbsolutePaths,
		false,
		true)

	if err != nil {
		return configAndStacksInfo, err
	}

	// Print the stack config files
	if g.LogVerbose {
		fmt.Println()
		var msg string
		if c.ProcessedConfig.StackType == "Directory" {
			msg = "Found the config file for the provided stack:"
		} else {
			msg = "Found config files:"
		}
		color.Cyan(msg)
		err = u.PrintAsYAML(c.ProcessedConfig.StackConfigFilesRelativePaths)
		if err != nil {
			return configAndStacksInfo, err
		}
	}

	if len(c.Config.Stacks.NamePattern) < 1 {
		return configAndStacksInfo,
			errors.New("stack name pattern must be provided in 'stacks.name_pattern' config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable")
	}

	stackNamePatternParts := strings.Split(c.Config.Stacks.NamePattern, "-")

	// Check and process stacks
	if c.ProcessedConfig.StackType == "Directory" {
		_, configAndStacksInfo.ComponentVarsSection, _, configAndStacksInfo.BaseComponentPath, configAndStacksInfo.Command, err = checkStackConfig(
			configAndStacksInfo.Stack,
			stacksMap,
			componentType,
			configAndStacksInfo.ComponentFromArg)
		if err != nil {
			return configAndStacksInfo, err
		}
	} else {
		if g.LogVerbose {
			color.Cyan("Searching for stack config where the component '%s' is defined\n", configAndStacksInfo.ComponentFromArg)
		}

		stackParts := strings.Split(configAndStacksInfo.Stack, "-")
		if len(stackParts) != len(stackNamePatternParts) {
			return configAndStacksInfo,
				errors.New(fmt.Sprintf("Stack '%s' does not match the stack name pattern '%s'",
					configAndStacksInfo.Stack,
					c.Config.Stacks.NamePattern))
		}

		var tenant string
		var environment string
		var stage string
		var tenantFound bool
		var environmentFound bool
		var stageFound bool

		for i, part := range stackNamePatternParts {
			if part == "{tenant}" {
				tenant = stackParts[i]
			} else if part == "{environment}" {
				environment = stackParts[i]
			} else if part == "{stage}" {
				stage = stackParts[i]
			}
		}

		for stackName := range stacksMap {
			_, configAndStacksInfo.ComponentVarsSection, _, configAndStacksInfo.BaseComponentPath, configAndStacksInfo.Command, err = checkStackConfig(
				stackName,
				stacksMap,
				componentType,
				configAndStacksInfo.ComponentFromArg)
			if err != nil {
				continue
			}

			tenantFound = true
			environmentFound = true
			stageFound = true

			// Search for tenant in stack
			if len(tenant) > 0 {
				if tenantInStack, ok := configAndStacksInfo.ComponentVarsSection["tenant"].(string); !ok || tenantInStack != tenant {
					tenantFound = false
				}
			}

			// Search for environment in stack
			if len(environment) > 0 {
				if environmentInStack, ok := configAndStacksInfo.ComponentVarsSection["environment"].(string); !ok || environmentInStack != environment {
					environmentFound = false
				}
			}

			// Search for stage in stack
			if len(stage) > 0 {
				if stageInStack, ok := configAndStacksInfo.ComponentVarsSection["stage"].(string); !ok || stageInStack != stage {
					stageFound = false
				}
			}

			if tenantFound == true && environmentFound == true && stageFound == true {
				if g.LogVerbose {
					color.Green("Found stack config for the component '%s' in the stack '%s'\n\n", configAndStacksInfo.ComponentFromArg, stackName)
				}
				configAndStacksInfo.Stack = stackName
				break
			}
		}

		if tenantFound == false || environmentFound == false || stageFound == false {
			return configAndStacksInfo,
				errors.New(fmt.Sprintf("\nCould not find config for the component '%s' in the stack '%s'.\n"+
					"Check that all attributes in the stack name pattern '%s' are defined in the stack config files.\n"+
					"Are the component and stack names correct? Did you forget an import?",
					configAndStacksInfo.ComponentFromArg,
					configAndStacksInfo.Stack,
					c.Config.Stacks.NamePattern,
				))
		}
	}

	if len(configAndStacksInfo.Command) == 0 {
		configAndStacksInfo.Command = componentType
	}

	color.Cyan("\nVariables for the component '%s' in the stack '%s':\n\n", configAndStacksInfo.ComponentFromArg, configAndStacksInfo.Stack)
	err = u.PrintAsYAML(configAndStacksInfo.ComponentVarsSection)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.ComponentFolderPrefix = ""
	configAndStacksInfo.ComponentNamePrefix = ""

	finalComponentPathParts := strings.Split(configAndStacksInfo.ComponentFromArg, "/")
	finalComponentPathPartsLength := len(finalComponentPathParts)

	if finalComponentPathPartsLength > 1 {
		componentFromArgPartsWithoutLast := finalComponentPathParts[:finalComponentPathPartsLength-1]
		configAndStacksInfo.ComponentFolderPrefix = strings.Join(componentFromArgPartsWithoutLast, "/")
		configAndStacksInfo.ComponentNamePrefix = strings.Join(componentFromArgPartsWithoutLast, "-")
		configAndStacksInfo.Component = finalComponentPathParts[finalComponentPathPartsLength-1]
	} else {
		configAndStacksInfo.Component = configAndStacksInfo.ComponentFromArg
	}

	if len(configAndStacksInfo.BaseComponentPath) > 0 {
		baseComponentPathParts := strings.Split(configAndStacksInfo.BaseComponentPath, "/")
		baseComponentPathPartsLength := len(baseComponentPathParts)
		if baseComponentPathPartsLength > 1 {
			configAndStacksInfo.BaseComponent = baseComponentPathParts[baseComponentPathPartsLength-1]
		} else {
			configAndStacksInfo.BaseComponent = configAndStacksInfo.BaseComponentPath
		}
	}

	// Process context
	configAndStacksInfo.Context = getContextFromVars(configAndStacksInfo.ComponentVarsSection)
	contextPrefix := ""

	for _, part := range stackNamePatternParts {
		if part == "{tenant}" {
			if len(configAndStacksInfo.Context.Tenant) == 0 {
				return configAndStacksInfo,
					errors.New(fmt.Sprintf("The stack name pattern '%s' specifies 'tenant`, but the stack %s does not have a tenant defined",
						c.Config.Stacks.NamePattern,
						configAndStacksInfo.Stack,
					))
			}
			if len(contextPrefix) == 0 {
				contextPrefix = configAndStacksInfo.Context.Tenant
			} else {
				contextPrefix = contextPrefix + "-" + configAndStacksInfo.Context.Tenant
			}
		} else if part == "{environment}" {
			if len(configAndStacksInfo.Context.Environment) == 0 {
				return configAndStacksInfo,
					errors.New(fmt.Sprintf("The stack name pattern '%s' specifies 'environment`, but the stack %s does not have an environment defined",
						c.Config.Stacks.NamePattern,
						configAndStacksInfo.Stack,
					))
			}
			if len(contextPrefix) == 0 {
				contextPrefix = configAndStacksInfo.Context.Environment
			} else {
				contextPrefix = contextPrefix + "-" + configAndStacksInfo.Context.Environment
			}
		} else if part == "{stage}" {
			if len(configAndStacksInfo.Context.Stage) == 0 {
				return configAndStacksInfo,
					errors.New(fmt.Sprintf("The stack name pattern '%s' specifies 'stage`, but the stack %s does not have a stage defined",
						c.Config.Stacks.NamePattern,
						configAndStacksInfo.Stack,
					))
			}
			if len(contextPrefix) == 0 {
				contextPrefix = configAndStacksInfo.Context.Stage
			} else {
				contextPrefix = contextPrefix + "-" + configAndStacksInfo.Context.Stage
			}
		}
	}

	configAndStacksInfo.ContextPrefix = contextPrefix
	return configAndStacksInfo, nil
}

// processArgsAndFlags removes common args and flags from the provided list of arguments/flags
func processArgsAndFlags(inputArgsAndFlags []string) (c.ArgsAndFlagsInfo, error) {
	var info c.ArgsAndFlagsInfo
	var additionalArgsAndFlags []string
	var globalOptions []string

	var indexesToRemove []int

	// https://github.com/roboll/helmfile#cli-reference
	var globalOptionsFlagIndex int

	for i, arg := range inputArgsAndFlags {
		if arg == g.GlobalOptionsFlag {
			globalOptionsFlagIndex = i + 1
		} else if strings.HasPrefix(arg+"=", g.GlobalOptionsFlag) {
			globalOptionsFlagIndex = i
		}

		if arg == g.TerraformDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.TerraformDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.TerraformDirFlag) {
			var terraformDirFlagParts = strings.Split(arg, "=")
			if len(terraformDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.TerraformDir = terraformDirFlagParts[1]
		}

		if arg == g.HelmfileDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.HelmfileDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.HelmfileDirFlag) {
			var helmfileDirFlagParts = strings.Split(arg, "=")
			if len(helmfileDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.HelmfileDir = helmfileDirFlagParts[1]
		}

		if arg == g.ConfigDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.StacksDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.ConfigDirFlag) {
			var configDirFlagParts = strings.Split(arg, "=")
			if len(configDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.StacksDir = configDirFlagParts[1]
		}

		if arg == g.StackDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.ConfigDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.StackDirFlag) {
			var stacksDirFlagParts = strings.Split(arg, "=")
			if len(stacksDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.ConfigDir = stacksDirFlagParts[1]
		}

		if arg == g.DeployRunInitFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.DeployRunInit = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.DeployRunInitFlag) {
			var deployRunInitFlagParts = strings.Split(arg, "=")
			if len(deployRunInitFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.DeployRunInit = deployRunInitFlagParts[1]
		}

		for _, f := range commonFlags {
			if arg == f {
				indexesToRemove = append(indexesToRemove, i)
				indexesToRemove = append(indexesToRemove, i+1)
			} else if strings.HasPrefix(arg, f+"=") {
				indexesToRemove = append(indexesToRemove, i)
			}
		}
	}

	for i, arg := range inputArgsAndFlags {
		if !u.SliceContainsInt(indexesToRemove, i) {
			additionalArgsAndFlags = append(additionalArgsAndFlags, arg)
		}

		if globalOptionsFlagIndex > 0 && i == globalOptionsFlagIndex {
			if strings.HasPrefix(arg, g.GlobalOptionsFlag+"=") {
				parts := strings.SplitN(arg, "=", 2)
				globalOptions = strings.Split(parts[1], " ")
			} else {
				globalOptions = strings.Split(arg, " ")
			}
		}
	}

	// Handle the legacy command `terraform write varfile`
	if additionalArgsAndFlags[0] == "write" && additionalArgsAndFlags[1] == "varfile" {
		info.SubCommand = "write varfile"
		info.ComponentFromArg = additionalArgsAndFlags[2]
		info.AdditionalArgsAndFlags = additionalArgsAndFlags[3:]
	} else {
		info.SubCommand = additionalArgsAndFlags[0]
		info.ComponentFromArg = additionalArgsAndFlags[1]
		info.AdditionalArgsAndFlags = additionalArgsAndFlags[2:]
	}

	info.GlobalOptions = globalOptions

	return info, nil
}

// execCommand prints and executes the provided command with args and flags
func execCommand(command string, args []string, dir string, env []string) error {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	color.Cyan("\nExecuting command:\n")
	fmt.Println(cmd.String())
	fmt.Println()
	return cmd.Run()
}

func getContextFromVars(vars map[interface{}]interface{}) c.Context {
	var context c.Context

	if namespace, ok := vars["namespace"].(string); ok {
		context.Namespace = namespace
	}

	if tenant, ok := vars["tenant"].(string); ok {
		context.Tenant = tenant
	}

	if environment, ok := vars["environment"].(string); ok {
		context.Environment = environment
	}

	if stage, ok := vars["stage"].(string); ok {
		context.Stage = stage
	}

	if region, ok := vars["region"].(string); ok {
		context.Region = region
	}

	return context
}

func replaceContextTokens(context c.Context, pattern string) string {
	return strings.Replace(
		strings.Replace(
			strings.Replace(
				strings.Replace(pattern,
					"{namespace}", context.Namespace, 1),
				"{environment}", context.Environment, 1),
			"{tenant}", context.Tenant, 1),
		"{stage}", context.Stage, 1)
}