package run

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/hashicorp/hcl"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	yaml "gopkg.in/yaml.v2"

	"github.com/ovh/venom"
	"github.com/ovh/venom/context/default"
	"github.com/ovh/venom/context/redis"
	"github.com/ovh/venom/context/webctx"

	osExec "os/exec"

	"github.com/ovh/venom/executors/dbfixtures"
	"github.com/ovh/venom/executors/exec"
	"github.com/ovh/venom/executors/http"
	"github.com/ovh/venom/executors/imap"
	"github.com/ovh/venom/executors/ovhapi"
	"github.com/ovh/venom/executors/readfile"
	"github.com/ovh/venom/executors/redis"
	"github.com/ovh/venom/executors/smtp"
	"github.com/ovh/venom/executors/ssh"
	"github.com/ovh/venom/executors/web"
)

var (
	path           []string
	variables      []string
	exclude        []string
	format         string
	varFile        string
	withEnv        bool
	logLevel       string
	outputDir      string
	detailsLevel   string
	resumeFailures bool
	resume         bool
	strict         bool
	noCheckVars    bool
	parallel       int
	stopOnFailure  bool
	v              *venom.Venom
)

func init() {
	Cmd.Flags().StringSliceVarP(&variables, "var", "", []string{""}, "--var cds='cds -f config.json' --var cds2='cds -f config.json'")
	Cmd.Flags().StringVarP(&varFile, "var-from-file", "", "", "--var-from-file filename.yaml : hcl|json|yaml, must contains map[string]string'")
	Cmd.Flags().StringSliceVarP(&exclude, "exclude", "", []string{""}, "--exclude filaA.yaml --exclude filaB.yaml --exclude fileC*.yaml")
	Cmd.Flags().StringVarP(&format, "format", "", "xml", "--format:yaml, json, xml, tap")
	Cmd.Flags().BoolVarP(&withEnv, "env", "", true, "Inject environment variables. export FOO=BAR -> you can use {{.FOO}} in your tests")
	Cmd.Flags().BoolVarP(&strict, "strict", "", false, "Exit with an error code if one test fails")
	Cmd.Flags().BoolVarP(&stopOnFailure, "stop-on-failure", "", false, "Stop running Test Suite on first Test Case failure")
	Cmd.Flags().BoolVarP(&noCheckVars, "no-check-variables", "", false, "Don't check variables before run")
	Cmd.Flags().IntVarP(&parallel, "parallel", "", 1, "--parallel=2 : launches 2 Test Suites in parallel")
	Cmd.PersistentFlags().StringVarP(&logLevel, "log", "", "warn", "Log Level : debug, info or warn")
	Cmd.PersistentFlags().StringVarP(&outputDir, "output-dir", "", "", "Output Directory: create tests results file inside this directory")
	Cmd.PersistentFlags().StringVarP(&detailsLevel, "details", "", "low", "Output Details Level : low, medium, high")
	Cmd.PersistentFlags().BoolVarP(&resume, "resume", "", false, "Output Resume: one line with Total, TotalOK, TotalKO, TotalSkipped, TotalTestSuite")
	Cmd.PersistentFlags().BoolVarP(&resumeFailures, "resumeFailures", "", false, "Output Resume Failures")
}

// Cmd run
var Cmd = &cobra.Command{
	Use:   "run",
	Short: "Run Tests",
	PreRun: func(cmd *cobra.Command, args []string) {

		if len(args) == 0 {
			path = append(path, ".")
		} else {
			path = args[0:]
		}

		v = venom.New()
		v.RegisterExecutor(exec.Name, exec.New())
		v.RegisterExecutor(http.Name, http.New())
		v.RegisterExecutor(imap.Name, imap.New())
		v.RegisterExecutor(readfile.Name, readfile.New())
		v.RegisterExecutor(smtp.Name, smtp.New())
		v.RegisterExecutor(ssh.Name, ssh.New())
		v.RegisterExecutor(web.Name, web.New())
		v.RegisterExecutor(ovhapi.Name, ovhapi.New())
		v.RegisterExecutor(dbfixtures.Name, dbfixtures.New())
		v.RegisterExecutor(redis.Name, redis.New())

		// Register Context
		v.RegisterTestCaseContext(defaultctx.Name, defaultctx.New())
		v.RegisterTestCaseContext(webctx.Name, webctx.New())
		v.RegisterTestCaseContext(redisctx.Name, redisctx.New())

	},
	Run: func(cmd *cobra.Command, args []string) {

		v.LogLevel = logLevel
		v.OutputDetails = detailsLevel
		v.OutputDir = outputDir
		v.OutputFormat = format
		v.OutputResume = resume
		v.OutputResumeFailures = resumeFailures
		v.Parallel = parallel
		v.StopOnFailure = stopOnFailure

		mapvars := make(map[string]string)
		if withEnv {
			variables = append(variables, os.Environ()...)
		}

		for _, a := range variables {
			t := strings.SplitN(a, "=", 2)
			if len(t) < 2 {
				continue
			}
			mapvars[t[0]] = strings.Join(t[1:], "")
		}

		if varFile != "" {
			varFileMap := make(map[string]string)
			bytes, err := ioutil.ReadFile(varFile)
			if err != nil {
				log.Fatal(err)
			}
			switch filepath.Ext(varFile) {
			case ".hcl":
				err = hcl.Unmarshal(bytes, &varFileMap)
			case ".json":
				err = json.Unmarshal(bytes, &varFileMap)
			case ".yaml", ".yml":
				err = yaml.Unmarshal(bytes, &varFileMap)
			default:
				log.Fatal("unsupported varFile format")
			}
			if err != nil {
				log.Fatal(err)
			}

			for key, value := range varFileMap {
				mapvars[key] = value
			}
		}

		v.AddVariables(mapvars)

		res := []*result{}

		suiteIndex := map[string]int{}
		caseIndex := map[string]int{}
		stepIndex := map[string]int{}

		s := spinner.New(spinner.CharSets[9], 100*time.Millisecond) // Build our new spinner
		s.Prefix = printRes(res, "")

		//s.Start()
		//	n := 0
		v.Hook = func(e venom.Event) {
			if e.Type == "testSuite" {
				var r *result
				if val, ok := suiteIndex[e.TestSuiteName]; ok {
					r = res[val]
				} else {
					r = &result{
						Res:  []*result{},
						Name: e.TestSuiteName,
					}
					suiteIndex[e.TestSuiteName] = len(res)
					res = append(res, r)
				}
				r.State = e.State
			} else if e.Type == "testCase" {
				i := suiteIndex[e.TestSuiteName]
				var r *result
				if val, ok := caseIndex[e.TestSuiteName+e.TestCaseName]; ok {
					r = res[i].Res[val]
				} else {
					r = &result{
						Res:  []*result{},
						Name: e.TestCaseName,
					}
					caseIndex[e.TestSuiteName+e.TestCaseName] = len(res[i].Res)
					res[i].Res = append(res[i].Res, r)
				}
				r.State = e.State
			} else if e.Type == "testStep" {
				i := suiteIndex[e.TestSuiteName]
				y := caseIndex[e.TestSuiteName+e.TestCaseName]

				var r *result
				if val, ok := stepIndex[e.TestSuiteName+e.TestCaseName+e.TestStepName]; ok {
					r = res[i].Res[y].Res[val]
				} else {
					r = &result{
						Res:  []*result{},
						Name: e.TestCaseName,
					}
					stepIndex[e.TestSuiteName+e.TestCaseName+e.TestStepName] = len(res[i].Res[y].Res)
					res[i].Res[y].Res = append(res[i].Res[y].Res, r)
				}
				r.State = e.State
			}

			fmt.Printf("\033[0;0H")
			o := printRes(res, "")
			fmt.Printf(o)

		}
		c := osExec.Command("clear")
		c.Stdout = os.Stdout
		c.Run()
		start := time.Now()

		if !noCheckVars {
			if err := v.Parse(path, exclude); err != nil {
				log.Fatal(err)
			}
		}

		tests, err := v.Process(path, exclude)
		if err != nil {
			log.Fatal(err)
		}
		s.Stop()

		elapsed := time.Since(start)
		if err := v.OutputResult(*tests, elapsed); err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			os.Exit(1)
		}
		if strict && tests.TotalKO > 0 {
			os.Exit(2)
		}
	},
}

func printRes(res []*result, prefix string) string {
	s := ""
	red := color.New(color.FgRed).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	for _, v := range res {
		if v.State == "SUCCESS" {
			v.State = green(v.State)
		} else if v.State == "FAILURE" {
			v.State = red(v.State)
		}
		s += fmt.Sprintf(prefix + v.State + " " + v.Name + "\n")
		s += printRes(v.Res, prefix+"    ")
	}
	return s
}

type result struct {
	Name  string
	State string
	Res   []*result
}
