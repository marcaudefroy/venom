package venom

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/fsamin/go-dump"
	"github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

func (v *Venom) initTestCaseContext(ts *TestSuite, tc *TestCase) (TestCaseContext, error) {
	var errContext error
	tc.Context, errContext = ts.Templater.ApplyOnContext(tc.Context)
	if errContext != nil {
		return nil, errContext
	}
	tcc, errContext := v.ContextWrap(tc)
	if errContext != nil {
		return nil, errContext
	}
	if err := tcc.Init(); err != nil {
		return nil, err
	}
	return tcc, nil
}

var varRegEx, _ = regexp.Compile("{{.*}}")

//Parse the testcase to find unreplaced and extracted variables
func (v *Venom) parseTestCase(ts *TestSuite, tc *TestCase) ([]string, []string, error) {
	tcc, err := v.initTestCaseContext(ts, tc)
	if err != nil {
		return nil, nil, err
	}
	defer tcc.Close()

	vars := []string{}
	extractedVars := []string{}

	for _, stepIn := range tc.TestSteps {
		step, erra := ts.Templater.ApplyOnStep(stepIn)
		if erra != nil {
			return nil, nil, erra
		}

		exec, err := v.WrapExecutor(step, tcc)
		if err != nil {
			return nil, nil, err
		}

		withZero, ok := exec.executor.(executorWithZeroValueResult)
		if ok {
			defaultResult := withZero.ZeroValueResult()
			dumpE, err := dump.ToStringMap(defaultResult, dump.WithDefaultLowerCaseFormatter())
			if err != nil {
				return nil, nil, err
			}

			for k := range dumpE {
				extractedVars = append(extractedVars, tc.Name+"."+k)
			}
		}

		dumpE, err := dump.ToStringMap(step, dump.WithDefaultLowerCaseFormatter())
		if err != nil {
			return nil, nil, err
		}

		for k, v := range dumpE {
			if strings.HasPrefix(k, "extracts.") {
				for _, extractVar := range extractPattern.FindAllString(v, -1) {
					varname := extractVar[2:strings.Index(extractVar, "=")]
					var found bool
					for i := 0; i < len(extractedVars); i++ {
						if extractedVars[i] == varname {
							found = true
							break
						}
					}
					if !found {
						extractedVars = append(extractedVars, tc.Name+"."+varname)
					}
				}
				continue
			}

			if varRegEx.MatchString(v) {
				var found bool
				for i := 0; i < len(vars); i++ {
					if vars[i] == k {
						found = true
						break
					}
				}

				for i := 0; i < len(extractedVars); i++ {
					s := varRegEx.FindString(v)
					prefix := "{{." + extractedVars[i]
					if strings.HasPrefix(s, prefix) {
						found = true
						break
					}
				}
				if !found {
					s := varRegEx.FindString(v)
					s = strings.Replace(s, "{{.", "", -1)
					s = strings.Replace(s, "}}", "", -1)
					vars = append(vars, s)
				}
			}
		}

	}
	return vars, extractedVars, nil
}

func (v *Venom) runTestCase(ts *TestSuite, tc *TestCase, l Logger) {
	start := time.Now()
	u1 := uuid.Must(uuid.NewV4())
	v.Hook(Event{
		State:         "RUN",
		Type:          "testCase",
		TestSuiteName: ts.Name,
		TestCaseName:  tc.Name + u1.String(),
	})
	tcc, err := v.initTestCaseContext(ts, tc)
	if err != nil {
		tc.Errors = append(tc.Errors, Failure{Value: RemoveNotPrintableChar(err.Error())})
		return
	}
	defer tcc.Close()

	if _l, ok := l.(*logrus.Entry); ok {
		l = _l.WithField("x.testcase", tc.Name)
	}
	l.Infof("start")

	for i, stepIn := range tc.TestSteps {
		v.Hook(Event{
			State:         "RUN",
			Type:          "testStep",
			TestSuiteName: ts.Name,
			TestCaseName:  tc.Name + u1.String(),
			TestStepName:  fmt.Sprintf("Step %d", i+1),
		})
		l.Debugf("Apply template on step %d", i)
		step, erra := ts.Templater.ApplyOnStep(stepIn)
		if erra != nil {
			tc.Errors = append(tc.Errors, Failure{Value: RemoveNotPrintableChar(erra.Error())})
			break
		}

		l.Debugf("Wrap executor on step %d", i)
		e, err := v.WrapExecutor(step, tcc)
		if err != nil {
			tc.Errors = append(tc.Errors, Failure{Value: RemoveNotPrintableChar(err.Error())})
			break
		}

		v.RunTestStep(tcc, e, ts, tc, step, l)
		//	var state string
		//	var final string
		if len(tc.Failures) > 0 || len(tc.Errors) > 0 {
			// red := color.New(color.FgRed).SprintFunc()
			// final = fmt.Sprintf(red("        FAILURE")+" step %d", i+1)
			//		fmt.Println(final)
			v.Hook(Event{
				State:         "FAILURE",
				Type:          "testStep",
				TestSuiteName: ts.Name,
				TestCaseName:  tc.Name + u1.String(),
				TestStepName:  fmt.Sprintf("Step %d", i+1),
			})
			break
		} else {
			v.Hook(Event{
				State:         "SUCCESS",
				Type:          "testStep",
				TestSuiteName: ts.Name,
				TestCaseName:  tc.Name + u1.String(),
				TestStepName:  fmt.Sprintf("Step %d", i+1),
			})
			// green := color.New(color.FgGreen).SprintFunc()
			// final = fmt.Sprintf(green("        SUCCESS")+" step %d", i+1)
			// fmt.Println(final)
		}

	}
	//var final string
	var state string
	if len(tc.Failures) > 0 || len(tc.Errors) > 0 {
		//	red := color.New(color.FgRed).SprintFunc()
		//	final = fmt.Sprintf(red("    FAILURE ") + tc.Name)
		state = "FAILURE"
	} else {
		//	green := color.New(color.FgGreen).SprintFunc()
		//		final = fmt.Sprintf(green("    SUCCESS ") + tc.Name)
		state = "SUCCESS"

	}
	//fmt.Println(final)

	v.Hook(Event{
		State:         state,
		Type:          "testCase",
		TestSuiteName: ts.Name,
		TestCaseName:  tc.Name + u1.String(),
	})

	elapsed := time.Since(start)
	tc.Time = fmt.Sprintf("%s", elapsed)
	l.Infof("end")
}
