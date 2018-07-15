package venom

import (
	"fmt"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/fsamin/go-dump"
	log "github.com/sirupsen/logrus"
)

func (v *Venom) runTestSuite(ts *TestSuite) {
	v.Hook(Event{
		State:         "RUN",
		Type:          "testSuite",
		TestSuiteName: ts.Name,
	})
	l := log.WithField("v.testsuite", ts.Name)
	start := time.Now()

	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond) // Build our new spinner
	s.Prefix = " " + ts.Name
	//s.Start()

	d, err := dump.ToStringMap(ts.Vars)
	if err != nil {
		log.Errorf("err:%s", err)
	}
	ts.Templater.Add("", d)

	totalSteps := 0
	for _, tc := range ts.TestCases {
		totalSteps += len(tc.TestSteps)
	}

	if v.OutputDetails != DetailsLow {
	}
	v.runTestCases(ts, l)

	elapsed := time.Since(start)

	ts.Time = fmt.Sprintf("%s", elapsed)

	var final string
	var state string
	if ts.Failures > 0 || ts.Errors > 0 {
		red := color.New(color.FgRed).SprintFunc()
		state = "FAILURE"
		final = fmt.Sprintf(red("FAILURE ") + ts.Name)
	} else {
		green := color.New(color.FgGreen).SprintFunc()
		state = "SUCCESS"

		final = fmt.Sprintf(green("SUCCESS ") + ts.Name)
	}

	s.FinalMSG = final
	v.Hook(Event{
		State:         state,
		Type:          "testSuite",
		TestSuiteName: ts.Name,
	})
	s.Stop()
	//	fmt.Println(final)

}

func (v *Venom) runTestCases(ts *TestSuite, l Logger) {
	for i := range ts.TestCases {
		tc := &ts.TestCases[i]
		if len(tc.Skipped) == 0 {
			v.runTestCase(ts, tc, l)
		}

		if len(tc.Failures) > 0 {
			ts.Failures += len(tc.Failures)
		}
		if len(tc.Errors) > 0 {
			ts.Errors += len(tc.Errors)
		}
		if len(tc.Skipped) > 0 {
			ts.Skipped += len(tc.Skipped)
		}

		if v.StopOnFailure && (len(tc.Failures) > 0 || len(tc.Errors) > 0) {
			// break TestSuite
			return
		}
	}
}

//Parse the suite to find unreplaced and extracted variables
func (v *Venom) parseTestSuite(ts *TestSuite) ([]string, []string, error) {
	d, err := dump.ToStringMap(ts.Vars)
	if err != nil {
		log.Errorf("err:%s", err)
	}
	ts.Templater.Add("", d)

	return v.parseTestCases(ts)
}

//Parse the testscases to find unreplaced and extracted variables
func (v *Venom) parseTestCases(ts *TestSuite) ([]string, []string, error) {
	vars := []string{}
	extractsVars := []string{}
	for i := range ts.TestCases {
		tc := &ts.TestCases[i]
		if len(tc.Skipped) == 0 {
			tvars, tExtractedVars, err := v.parseTestCase(ts, tc)
			if err != nil {
				return nil, nil, err
			}
			for _, k := range tvars {
				var found bool
				for i := 0; i < len(vars); i++ {
					if vars[i] == k {
						found = true
						break
					}
				}
				if !found {
					vars = append(vars, k)
				}
			}
			for _, k := range tExtractedVars {
				var found bool
				for i := 0; i < len(extractsVars); i++ {
					if extractsVars[i] == k {
						found = true
						break
					}
				}
				if !found {
					extractsVars = append(extractsVars, k)
				}
			}
		}
	}

	return vars, extractsVars, nil
}
