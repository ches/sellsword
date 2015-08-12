package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"strings"
)

var log = logrus.New()

type sswEnv struct {
	Name      string
	EnvType   string `yaml:"type"`
	Path      string
	Variables []string
}

func (c *sswEnv) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

func (c *sswEnv) ParseExportVars() (map[string]string, error) {
	exportVars := make(map[string]string)
	for i := range c.Variables {
		keyValue := strings.Split(c.Variables[i], "=")
		exportVars[keyValue[1]] = keyValue[0]
	}
	return exportVars, nil
}

func getTermPrinter(colorName color.Attribute) func(...interface{}) string {
	newColor := color.New(colorName)
	newColor.EnableColor()
	return newColor.SprintFunc()
}

func mergeEnvMap(dest map[string]string, src map[string]string) {
	for key, value := range src {
		if _, ok := dest[key]; ok {
			log.Debugf("There is already a value present for %s, ignoring new value", key)
		} else {
			dest[key] = value
		}
	}
}

func populateExportVars(exportVars map[string]string, currentVars map[string]string) {
	for key, value := range exportVars {
		if currentValue, ok := currentVars[value]; ok {
			exportVars[key] = currentValue
		} else {
			delete(exportVars, key)
		}
	}
}

func convertToBash(exportVars map[string]string) {
	exports := make([]string, 1)
	for key, value := range exportVars {
		exports = append(exports, "export "+key+"="+value)
	}
	fmt.Println(strings.Join(exports, "\n"))
}

func parseEnvName(pathName string) string {
	envName := strings.Split(path.Base(pathName), ".ssw")[0]
	return strings.Split(envName, "-env")[0]
}

func listEnv(env *sswEnv) {
	cyan := getTermPrinter(color.FgCyan)
	fmt.Printf("%s:\n", cyan(env.Name))
	currentEnv := ""
	if env.Path == "" {
		red := getTermPrinter(color.FgRed)
		fmt.Printf("%s\n", red("No environment currently in use"))
	} else {
		currentEnv = parseEnvName(env.Path)
		log.Debugf("currentEnv is %s", currentEnv)

		green := getTermPrinter(color.FgGreen)
		fmt.Printf("\t%s\t%s\n", green(currentEnv), green("CURRENT"))
	}
	usr, _ := user.Current()
	appdir := path.Join(usr.HomeDir, "/.ssw/", env.Name)
	dirInfo, _ := ioutil.ReadDir(appdir)
	var envName string
	for i := range dirInfo {
		envName = parseEnvName(dirInfo[i].Name())
		if envName != "current" && envName != currentEnv {
			fmt.Printf("\t%s\n", envName)
		}
	}
}

func listEnvs(envList []string) {
	envs, _ := findCurrentEnvs()
	if len(envList) == 0 {
		for k, _ := range envs {
			listEnv(envs[k])
		}
	} else {
		for i := range envList {
			if env, ok := envs[envList[i]]; ok {
				listEnv(env)
			} else {
				apps := make([]string, 0, len(envs))
				for k := range envs {
					apps = append(apps, k)
				}
				log.Errorf("There is no configuration for %s, configurations are available for %s",
					envList[i], strings.Join(apps, ", "))
			}
		}
	}
}

func showCurrentEnvs() {
	envs, _ := findCurrentEnvs()
	fmt.Println("Environments in use:")
	for k, _ := range envs {
		if envs[k].Path != "" {
			currentEnv := strings.Split(path.Base(envs[k].Path), ".ssw")[0]
			currentEnv = strings.Split(currentEnv, "-env")[0]
			green := getTermPrinter(color.FgGreen)
			blue := getTermPrinter(color.FgCyan)
			fmt.Printf("%s\t%s\n", green(envs[k].Name), blue(currentEnv))
		}
	}
}

func readCurrentEnv(currentPath string, env *sswEnv) {
	fi, err := os.Lstat(currentPath)
	if err != nil {
		log.Errorf("Path %s does not exist\n", currentPath)
	} else {
		if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
			realPath, err := os.Readlink(currentPath)
			log.Debugf("Resolved current env for %s to %s\n", env.Name, realPath)
			if err == nil {
				env.Path = realPath
			} else {
				env.Path = ""
			}
		} else {
			env.Path = ""
			log.Errorf("Path %s is not a symlink and should be", currentPath)
		}
	}
}

func findCurrentEnvs() (map[string]*sswEnv, error) {
	envs, nil := findEnvs()
	usr, _ := user.Current()
	homedir := path.Join(usr.HomeDir, "/.ssw/")
	for k, _ := range envs {
		if envs[k].EnvType == "environment" {
			currentEnvPath := path.Join(homedir, envs[k].Name, "current-env.ssw")
			readCurrentEnv(currentEnvPath, envs[k])
		} else if envs[k].EnvType == "directory" {
			currentEnvPath := path.Join(homedir, envs[k].Name, "current")
			readCurrentEnv(currentEnvPath, envs[k])
		}
	}
	return envs, nil
}

func findEnvs() (map[string]*sswEnv, error) {
	envs := make(map[string]*sswEnv)
	usr, _ := user.Current()
	homedir := path.Join(usr.HomeDir, "/.ssw/config")
	log.Debugln(homedir)
	dirInfo, err := ioutil.ReadDir(homedir)
	envFiles := make([]string, 0)
	if err == nil {
		for i := range dirInfo {
			if strings.HasSuffix(dirInfo[i].Name(), ".ssw") {
				log.Debugf("Found configuration file " + dirInfo[i].Name())
				envFiles = append(envFiles, path.Join(homedir, dirInfo[i].Name()))
			}
		}
	} else {
		log.Error("Failed to read directory " + homedir + ". Received error " + err.Error())
	}
	for i := range envFiles {
		data, err := ioutil.ReadFile(envFiles[i])
		if err != nil {
			log.Error(err)
		}
		env := new(sswEnv)
		env.Name = strings.TrimSuffix(path.Base(envFiles[i]), ".ssw")
		if err := env.Parse(data); err != nil {
			log.Error(err)
		}
		envs[env.Name] = env
	}
	return envs, nil
}

func loadEnvs() {
	allExportedVars := make(map[string]string)
	envs, _ := findCurrentEnvs()
	for k, _ := range envs {
		if envs[k].EnvType == "environment" {
			exportVars, err := envs[k].ParseExportVars()
			if err == nil {
				log.Debugf("Loading current environment %s", envs[k].Path)
				currentVars := make(map[string]string)
				envData, err := ioutil.ReadFile(envs[k].Path)
				if err == nil {
					yaml.Unmarshal(envData, currentVars)
					populateExportVars(exportVars, currentVars)
					mergeEnvMap(allExportedVars, exportVars)
				} else {
					log.Error(err)
				}
			}
		}
	}
	convertToBash(allExportedVars)
}

func mkdirP(directories []string) {
	for dir := range directories {
		_, stat_err := os.Stat(directories[dir])
		if stat_err != nil {
			if _, ok := stat_err.(*os.PathError); ok {
				mkdir_err := os.MkdirAll(directories[dir], 0755)
				if mkdir_err != nil {
					log.Error("Received error " + mkdir_err.Error())
					os.Exit(0)
				} else {
					log.Println("Created directory " + directories[dir])
				}
			} else {
				log.Error("Received error " + stat_err.Error())
				os.Exit(0)
			}
		} else {
			log.Println("Directory ", directories[dir], "already exists")
		}
	}
}

func main() {
	log.Out = os.Stderr
	formatter := &logrus.TextFormatter{}
	formatter.ForceColors = true
	log.Formatter = formatter
	log.Level = logrus.InfoLevel
	var Verbose bool

	var sswCmd = &cobra.Command{
		Use:   "sellsword",
		Short: "Sellsword is a generic command-line tool for switching between application configurations",
		Long:  `Sellsword is a generic tool for switching between application configurations`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Please invoke one of Sellsword's subcommands to get started. Type sellsword help for more information")
		},
	}
	sswCmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false, "verbose output")

	sswCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		// for some reason I have to look up the verbose flag rather than just access the Verbose var
		v := sswCmd.Flags().Lookup("verbose").Value.String()
		if v == "true" {
			log.Level = logrus.DebugLevel
		}
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number of sellsword",
		Long:  `All software has versions. This is Sellsword's`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Sellsword version 0.0.1")
		},
	}
	sswCmd.AddCommand(versionCmd)

	var initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize Sellsword",
		Long:  `This command creates the Sellsword directory structure and downloads default configurations from git@github.com:bryanwb/sellsword.git`,
		Run: func(cmd *cobra.Command, args []string) {
			usr, _ := user.Current()
			homedir := path.Join(usr.HomeDir, ".ssw/config")
			awsdir := path.Join(usr.HomeDir, ".ssw/aws")
			chefdir := path.Join(usr.HomeDir, ".ssw/chef")
			mkdirP([]string{homedir, awsdir, chefdir})
		},
	}
	sswCmd.AddCommand(initCmd)

	var loadCmd = &cobra.Command{
		Use:   "load",
		Short: "Loads current Sellsword configurations",
		Long:  `This command loads all default environment configurations for use by the shell`,
		Run: func(cmd *cobra.Command, args []string) {
			loadEnvs()
		},
	}
	sswCmd.AddCommand(loadCmd)

	var showCmd = &cobra.Command{
		Use:   "show",
		Short: "Show Sellsword environments",
		Long:  `Show current Sellsword environments`,
		Run: func(cmd *cobra.Command, args []string) {
			showCurrentEnvs()
		},
	}
	sswCmd.AddCommand(showCmd)

	var listCmd = &cobra.Command{
		Use:   "list [env ...]",
		Short: "list available Sellsword environments",
		Long:  `List available Sellsword environments`,
		Run: func(cmd *cobra.Command, args []string) {
			listEnvs(args)
		},
	}
	sswCmd.AddCommand(listCmd)

	sswCmd.Execute()

}
