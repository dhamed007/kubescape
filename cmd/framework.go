package cmd

import (
	"errors"
	"flag"
	"fmt"
	"kubescape/cautils"
	"kubescape/cautils/armotypes"
	"kubescape/cautils/k8sinterface"
	"kubescape/cautils/opapolicy"
	"kubescape/opaprocessor"
	"kubescape/policyhandler"
	"kubescape/printer"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var scanInfo opapolicy.ScanInfo
var supportedFrameworks = []string{"nsa", "mitre"}

type CLIHandler struct {
	policyHandler *policyhandler.PolicyHandler
	scanInfo      *opapolicy.ScanInfo
}

var frameworkCmd = &cobra.Command{
	Use:       "framework <framework name>",
	Short:     fmt.Sprintf("The framework you wish to use. Supported frameworks: %s", strings.Join(supportedFrameworks, ", ")),
	Long:      ``,
	ValidArgs: supportedFrameworks,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires at least one argument")
		}
		if !isValidFramework(args[0]) {
			return errors.New(fmt.Sprintf("supported frameworks: %s", strings.Join(supportedFrameworks, ", ")))
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		scanInfo.PolicyIdentifier = opapolicy.PolicyIdentifier{}
		scanInfo.PolicyIdentifier.Kind = opapolicy.KindFramework
		scanInfo.PolicyIdentifier.Name = args[0]
		scanInfo.InputPatterns = args[1:]
		cautils.SetSilentMode(scanInfo.Silent)
		CliSetup()
	},
}

func isValidFramework(framework string) bool {
	return cautils.StringInSlice(supportedFrameworks, framework) != cautils.ValueNotFound
}

func init() {
	scanCmd.AddCommand(frameworkCmd)
	scanInfo = opapolicy.ScanInfo{}
	frameworkCmd.Flags().StringVarP(&scanInfo.ExcludedNamespaces, "exclude-namespaces", "e", "", "namespaces to exclude from check")
	frameworkCmd.Flags().StringVarP(&scanInfo.Output, "output", "o", "pretty-printer", "output format. supported formats: 'pretty-printer'/'json'/'junit'")
	frameworkCmd.Flags().BoolVarP(&scanInfo.Silent, "silent", "s", false, "silent progress output")
}

func CliSetup() error {
	flag.Parse()

	k8s := k8sinterface.NewKubernetesApi()

	processNotification := make(chan *cautils.OPASessionObj)
	reportResults := make(chan *cautils.OPASessionObj)

	// policy handler setup
	policyHandler := policyhandler.NewPolicyHandler(&processNotification, k8s)

	// cli handler setup
	cli := NewCLIHandler(policyHandler)
	if err := cli.Scan(); err != nil {
		panic(err)
	}

	// processor setup - rego run
	go func() {
		reporterObj := opaprocessor.NewOPAProcessor(&processNotification, &reportResults)
		reporterObj.ProcessRulesListenner()
	}()
	p := printer.NewPrinter(&reportResults, scanInfo.Output)
	p.ActionPrint()

	return nil
}

func NewCLIHandler(policyHandler *policyhandler.PolicyHandler) *CLIHandler {
	return &CLIHandler{
		scanInfo:      &scanInfo,
		policyHandler: policyHandler,
	}
}

func (clihandler *CLIHandler) Scan() error {
	cautils.ScanStartDisplay()
	policyNotification := &opapolicy.PolicyNotification{
		NotificationType: opapolicy.TypeExecPostureScan,
		Rules: []opapolicy.PolicyIdentifier{
			clihandler.scanInfo.PolicyIdentifier,
		},
		Designators: armotypes.PortalDesignator{},
	}
	switch policyNotification.NotificationType {
	case opapolicy.TypeExecPostureScan:
		go func() {
			if err := clihandler.policyHandler.HandleNotificationRequest(policyNotification, clihandler.scanInfo); err != nil {
				fmt.Printf("%v\n", err)
				os.Exit(0)
			}
		}()
	default:
		return fmt.Errorf("notification type '%s' Unknown", policyNotification.NotificationType)
	}
	return nil
}
