// This migration tool allows converting the Falco regression tests
// executed by falco_tests.py (falco_tests.yaml, falco_tests_exceptions, ...),
// into a format compatible with this Go framework.
// Some of those tests (~12 of them) require manual intervention for different
// reasons, but the bulk of the old tests is compatible with the framework

package main

import (
	"fmt"
	"os"
	"path"
	"strings"
	"text/template"
	"time"

	"github.com/iancoleman/strcase"
	"gopkg.in/yaml.v3"
)

// tests that require manual intervention
var problematicTests = []string{
	"Yes", "No", // these are just parsing leftovers
	"InOperatorNetmasks",
	"InvalidMacroLoop",
	"EnabledRuleUsingFalseEnabledFlagOnly", // needs reworking to check rule name instead of stdout regexp
	"JsonOutputNoTagsProperty",             // needs reworking to use json stdout
	"NullOutputField",                      // needs reworking to use json stdout
	"JsonOutputNoOutputProperty",           // needs reworking to use json stdout
	"TimeIso8601",                          // needs reworking to use json stdout
	"JsonOutputEmptyTagsProperty",          // json_include_tags_property=true must be true for some reason
	"RuleNamesWithRegexChars",              // rule is matched with regex
	"DetectCounts",                         // scap file wrong name
	"RulesDirectory",                       // rules files wrong name
	"TestWarnings",
	"GrpcUnixSocketOutputs",
	"TestKubeDemo", // it works but needs a 30secs timeout (and running go test with a custom -timeout flag)
}

func die(err error) {
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}
}

type TestTemplateTestInput struct {
	Name    string
	Options []string
	Checks  []string
}

type TestTemplateInput struct {
	Tests       []TestTemplateTestInput
	Timestamp   time.Time
	PackageName string
}

var testTemplate = template.Must(template.New("goTest").Parse(`// Code generated by go generate; DO NOT EDIT.
// This file was generated by robots at {{ .Timestamp }}

package {{ .PackageName }}

import (
	"testing"

	"github.com/jasondellaluce/falco-testing/pkg/falco"
	"github.com/jasondellaluce/falco-testing/tests/falco/data/rules"
    "github.com/jasondellaluce/falco-testing/tests/falco/data/configs"
    "github.com/jasondellaluce/falco-testing/tests/falco/data/captures"
	"github.com/stretchr/testify/assert"
)
{{range $testIndex, $test := .Tests}}
func TestLegacy_{{ $test.Name }}(t *testing.T) {
	t.Parallel()
    res := falco.Test(
        newExecutableRunner(t),{{range $optionIndex, $option := $test.Options}}
        {{ $option }},{{end}}
    ){{range $checkIndex, $check := $test.Checks}}
    {{ $check }}{{end}}
}
{{end}}
`))

// note: can be string or []string
type singleOrMultiString interface{}

func MultiStrValues(s singleOrMultiString) []string {
	if str, ok := s.(string); ok {
		return []string{str}
	}
	if strs, ok := s.([]string); ok {
		return strs
	}
	if strs, ok := s.([]interface{}); ok {
		var res []string
		for _, str := range strs {
			res = append(res, str.(string))
		}
		return res
	}
	return []string{}
}

func quoteString(s string) string {
	return `"` + s + `"`
}

func convertStrings(strs []string, f func(string) string) []string {
	var res []string
	for _, s := range strs {
		res = append(res, f(s))
	}
	return res
}

type FalcoTestInfo struct {
	AddlCmdlineOpts           string              `yaml:"addl_cmdline_opts"`
	Detect                    bool                `yaml:"detect"`
	DisableTags               []string            `yaml:"disable_tags"`
	RunTags                   []string            `yaml:"run_tags"`
	DisabledRules             []string            `yaml:"disabled_rules"`
	TraceFile                 string              `yaml:"trace_file"`
	AllEvents                 bool                `yaml:"all_events"`
	CheckDetectionCounts      bool                `yaml:"check_detection_counts"`
	EnableSource              singleOrMultiString `yaml:"enable_source"`
	ValidateRulesFile         singleOrMultiString `yaml:"validate_rules_file"`
	ConfFile                  string              `yaml:"conf_file"`
	RulesFile                 singleOrMultiString `yaml:"rules_file"`
	RunDuration               int                 `yaml:"run_duration"`
	StderrContains            singleOrMultiString `yaml:"stderr_contains"`
	StderrNotContains         singleOrMultiString `yaml:"stderr_not_contains"`
	StdoutContains            singleOrMultiString `yaml:"stdout_contains"`
	StdoutNotContains         singleOrMultiString `yaml:"stdout_not_contains"`
	TimeIso8601               bool                `yaml:"time_iso_8601"`
	JSONIncludeOutputProperty bool                `yaml:"json_include_output_property"`
	JSONIncludeTagsProperty   bool                `yaml:"json_include_tags_property"`
	JSONOutput                bool                `yaml:"json_output"`
	ValidateOk                []string            `yaml:"validate_ok"`
	ValidateWarnings          []struct {
		ItemType string `yaml:"item_type"`
		ItemName string `yaml:"item_name"`
		Code     string `yaml:"code"`
		Message  string `yaml:"message"`
	} `yaml:"validate_warnings"`
	DetectLevel    singleOrMultiString `yaml:"detect_level"`
	Priority       string              `yaml:"priority"`
	DetectCounts   []map[string]int    `yaml:"detect_counts"`
	ExitStatus     int                 `yaml:"exit_status"`
	ValidateErrors []struct {
		ItemType string `yaml:"item_type"`
		ItemName string `yaml:"item_name"`
		Code     string `yaml:"code"`
		Message  string `yaml:"message"`
	} `yaml:"validate_errors"`
	// note: the ones below are ignored for now
	RulesEvents interface{} `yaml:"rules_events"`
	Grpc        interface{} `yaml:"grpc"`
	Package     interface{} `yaml:"package"`
}

type FalcoTestConfig map[string]map[string]FalcoTestInfo

func filePackageName(packageName string) func(string) string {
	return func(s string) string {
		noExtension := strings.TrimSuffix(s, path.Ext(s))
		for _, pr := range []string{"rules/", "trace_files/", "confs/"} {
			noExtension = strings.TrimPrefix(noExtension, pr)
		}
		snakeCase := strings.ReplaceAll(noExtension, "/", "_")
		return packageName + "." + strcase.ToCamel(snakeCase)
	}
}

func (f FalcoTestConfig) TestInputs() []TestTemplateTestInput {
	var res []TestTemplateTestInput
	for _, testsInfo := range f {
		for testName, testsInfo := range testsInfo {
			t, ok := testsInfo.TemplateInput(strcase.ToCamel(testName))
			if ok {
				res = append(res, t)
			}
		}
	}
	return res
}

func (f FalcoTestInfo) TemplateInput(name string) (TestTemplateTestInput, bool) {
	res := TestTemplateTestInput{Name: name}
	for _, prob := range problematicTests {
		if name == prob {
			println("skipping test:", name)
			// test requires manual intervention
			return res, false
		}
	}
	if f.Grpc != nil || f.Package != nil || f.RulesEvents != nil {
		// ignoring these tests for now, they require manual intervention
		println("skipping test:", name)
		return res, false
	}
	cmdValidation := len(f.ValidateErrors) > 0 ||
		len(f.ValidateWarnings) > 0 ||
		len(MultiStrValues(f.ValidateRulesFile)) > 0
	cmdDetect := f.Detect ||
		f.CheckDetectionCounts ||
		len(MultiStrValues(f.DetectLevel)) > 0 ||
		len(f.DetectCounts) > 0

	if len(f.AddlCmdlineOpts) > 0 {
		cmds := convertStrings(strings.Split(f.AddlCmdlineOpts, " "), quoteString)
		res.Options = append(res.Options, `falco.WithArgs(`+strings.Join(cmds, ", ")+`)`)
	}
	if len(f.Priority) > 0 {
		res.Options = append(res.Options, `falco.WithMinRulePriority("`+f.Priority+`")`)
	}
	if f.RunDuration > 0 {
		res.Options = append(res.Options, fmt.Sprintf(`falco.WithMaxDuration(%d * time.Second)`, f.RunDuration))
	}
	if len(f.ConfFile) > 0 {
		res.Options = append(res.Options, `falco.WithConfig(`+filePackageName("configs")(f.ConfFile)+`)`)
	}
	if len(MultiStrValues(f.StderrContains)) > 0 {
		for _, v := range MultiStrValues(f.StderrContains) {
			res.Checks = append(res.Checks, "assert.Regexp(t, `"+v+"`, res.Stderr())")
		}
	}
	if len(MultiStrValues(f.StderrNotContains)) > 0 {
		for _, v := range MultiStrValues(f.StderrNotContains) {
			res.Checks = append(res.Checks, "assert.NotRegexp(t, `"+v+"`, res.Stderr())")
		}
	}
	if len(MultiStrValues(f.StdoutContains)) > 0 {
		for _, v := range MultiStrValues(f.StdoutContains) {
			res.Checks = append(res.Checks, "assert.Regexp(t, `"+v+"`, res.Stdout())")
		}
	}
	if len(MultiStrValues(f.StdoutNotContains)) > 0 {
		for _, v := range MultiStrValues(f.StdoutNotContains) {
			res.Checks = append(res.Checks, "assert.NotRegexp(t, `"+v+"`, res.Stdout())")
		}
	}
	if f.JSONOutput || cmdValidation || cmdDetect {
		res.Options = append(res.Options, `falco.WithOutputJSON()`)
	}
	if len(MultiStrValues(f.RulesFile)) > 0 {
		values := convertStrings(MultiStrValues(f.RulesFile), filePackageName("rules"))
		res.Options = append(res.Options, `falco.WithRules(`+strings.Join(values, ", ")+`)`)
	}
	if len(f.DisabledRules) > 0 {
		values := convertStrings(f.DisabledRules, quoteString)
		res.Options = append(res.Options, `falco.WithDisabledRules(`+strings.Join(values, ", ")+`)`)
	}
	if len(f.DisableTags) > 0 {
		tags := convertStrings(f.DisableTags, quoteString)
		res.Options = append(res.Options, `falco.WithDisabledTags(`+strings.Join(tags, ", ")+`)`)
	}
	if len(f.RunTags) > 0 {
		tags := convertStrings(f.RunTags, quoteString)
		res.Options = append(res.Options, `falco.WithEnabledTags(`+strings.Join(tags, ", ")+`)`)
	}
	if len(f.TraceFile) > 0 && !cmdValidation {
		name := filePackageName("captures")(f.TraceFile)
		res.Options = append(res.Options, `falco.WithCaptureFile(`+name+`)`)
	}
	if cmdValidation {
		if len(MultiStrValues(f.ValidateRulesFile)) > 0 {
			values := convertStrings(MultiStrValues(f.ValidateRulesFile), filePackageName("rules"))
			res.Options = append(res.Options, `falco.WithRulesValidation(`+strings.Join(values, ", ")+`)`)
		}
		for _, rule := range f.ValidateOk {
			idx := -1
			for i, vrule := range MultiStrValues(f.ValidateRulesFile) {
				if strings.Contains(vrule, rule) {
					idx = i
				}
			}
			if idx < 0 {
				die(fmt.Errorf("text not well-formed: validate_ok refs unknown rules file: " + name))
			}
			res.Checks = append(res.Checks, fmt.Sprintf(`assert.True(t, res.RuleValidation().ForIndex(%d).Successful)`, idx))
		}
		for _, info := range f.ValidateErrors {
			check := "assert.NotNil(t, res.RuleValidation().AllErrors()"
			if len(info.Code) > 0 {
				check += ".\n        ForCode(\"" + info.Code + "\")"
			}
			if len(info.ItemType) > 0 {
				check += ".\n        ForItemType(\"" + info.ItemType + "\")"
			}
			if len(info.ItemName) > 0 {
				check += ".\n        ForItemName(\"" + info.ItemName + "\")"
			}
			if len(info.Message) > 0 {
				check += ".\n        ForMessage(\"" + info.Message + "\")"
			}
			res.Checks = append(res.Checks, check+")")
		}
		for _, info := range f.ValidateWarnings {
			check := "assert.NotNil(t, res.RuleValidation().AllWarnings()"
			if len(info.Code) > 0 {
				check += ".\n        ForCode(\"" + info.Code + "\")"
			}
			if len(info.ItemType) > 0 {
				check += ".\n        ForItemType(\"" + info.ItemType + "\")"
			}
			if len(info.ItemName) > 0 {
				check += ".\n        ForItemName(\"" + info.ItemName + "\")"
			}
			if len(info.Message) > 0 {
				check += ".\n        ForMessage(\"" + info.Message + "\")"
			}
			res.Checks = append(res.Checks, check+")")
		}
	}
	if cmdDetect {
		if f.AllEvents {
			res.Options = append(res.Options, `falco.WithAllEvents()`)
		}
		if f.TimeIso8601 {
			// todo(jasondellaluce): consider removing this
			res.Options = append(res.Options, `falco.WithArgs("-o", "time_format_iso_8601=true")`)
		}
		if len(MultiStrValues(f.EnableSource)) > 0 {
			sources := convertStrings(MultiStrValues(f.EnableSource), quoteString)
			res.Options = append(res.Options, `falco.WithEnabledSources(`+strings.Join(sources, ", ")+`)`)
		}
		if !f.Detect {
			res.Checks = append(res.Checks, `assert.Zero(t, res.Detections().Count())`)
		} else {
			res.Checks = append(res.Checks, `assert.NotZero(t, res.Detections().Count())`)
		}
		for _, level := range MultiStrValues(f.DetectLevel) {
			if f.Detect {
				res.Checks = append(res.Checks, `assert.NotZero(t, res.Detections().ForPriority("`+level+`").Count())`)
			} else {
				res.Checks = append(res.Checks, `assert.Zero(t, res.Detections().ForPriority("`+level+`").Count())`)
			}
		}
		for _, check := range f.DetectCounts {
			for rule, count := range check {
				res.Checks = append(res.Checks, fmt.Sprintf(`assert.Equal(t, %d, res.Detections().ForRule("%s").Count())`, count, rule))
			}
		}
		// todo(jasondellaluce): consider removing this
		res.Options = append(res.Options, fmt.Sprintf(`falco.WithArgs("-o", "json_include_output_property=%v")`, f.JSONIncludeOutputProperty))
		// todo(jasondellaluce): consider removing this
		res.Options = append(res.Options, fmt.Sprintf(`falco.WithArgs("-o", "json_include_tags_property=%v")`, f.JSONIncludeTagsProperty))

	}
	if f.ExitStatus != 0 {
		res.Checks = append(res.Checks, `assert.NotNil(t, res.Err())`)
	} else {
		res.Checks = append(res.Checks, ` assert.Nil(t, res.Err(), "%s", res.Stderr())`)
	}
	res.Checks = append(res.Checks, fmt.Sprintf(`assert.Equal(t, %d, res.ExitCode())`, f.ExitStatus))
	return res, true
}

func readConfig(file string) (FalcoTestConfig, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	res := make(FalcoTestConfig)
	err = yaml.NewDecoder(f).Decode(&res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func main() {
	files := []string{
		"falco_tests.yaml",
		//"falco_k8s_audit_tests.yaml",
		"falco_tests_exceptions.yaml",
		//"falco_tests_package.yaml",
		"falco_traces.yaml",
	}

	input := TestTemplateInput{
		Timestamp:   time.Now(),
		PackageName: "tests",
	}
	for _, fname := range files {
		config, err := readConfig("./generated/falco-0.33.1/test/" + fname)
		die(err)
		input.Tests = append(input.Tests, config.TestInputs()...)
	}

	die(testTemplate.Execute(os.Stdout, input))
}
