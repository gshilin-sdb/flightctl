package cli_test

import (
	"crypto/rand"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flightctl/flightctl/test/harness/e2e"
	"github.com/flightctl/flightctl/test/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	"github.com/sirupsen/logrus"
)

const TIMEOUT = "1m"
const POLLING = "250ms"

func TestCLI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CLI E2E Suite")
}

var _ = Describe("cli operation", func() {
	var (
		harness *e2e.Harness
		token   string
	)

	BeforeEach(func() {
		harness = e2e.NewTestHarness()

		// construct the flightctl login arguments
		loginArgs := []string{"login", "${API_ENDPOINT}", "--insecure-skip-tls-verify"}
		if token != "" {
			loginArgs = append(loginArgs, "--token", token)
		}
		// attempt login
		out, err := harness.CLI(loginArgs...)

		// if openshift authentication is required, try to obtain a token
		if strings.Contains(out, "You must obtain an API token by visiting") {
			token, err = harness.SH("oc", "whoami", "-t")
			token = strings.Trim(token, "\n")
			Expect(err).ToNot(HaveOccurred())
			Expect(token).ToNot(BeEmpty(), "token from oc whoami should not be empty")
			loginArgs = append(loginArgs, "--token", token)
			out, err = harness.CLI(loginArgs...)

		}
		Expect(err).ToNot(HaveOccurred())
		out = strings.Trim(out, "\n.")
		Expect(out).To(BeElementOf("Auth is disabled", "Login successful"))
	})

	AfterEach(func() {
		harness.Cleanup(false) // do not print console on error
	})

	Context("login", func() {
		It("should have worked, and we can list devices", func() {
			out, err := harness.CLI("get", "devices")
			Expect(err).ToNot(HaveOccurred())
			Expect(out).To(ContainSubstring("NAME"))
		})
	})

	Context("apply/recursive", func() {
		It("should work for a complete set of yamls", func() {
			out, err := harness.CLI("apply", "-R", "-f", filepath.Join(util.GetTopLevelDir(), "examples"))
			Expect(err).ToNot(HaveOccurred())
			// expect out to contain 200 OK or 201 Created
			Expect(out).To(MatchRegexp(`(200 OK|201 Created)`))

			// check a for a couple of the yamls we know to exist in the examples directory
			Expect(out).To(ContainSubstring("examples/device-standalone.yaml/f68dfb5f5d2cdbb9339363b7f19f3ce269d75650bdc80004f1e04293a8ef9c4"))
			Expect(out).To(ContainSubstring("examples/resourcesync.yaml/default-sync"))
		})
	})

	Context("apply/fleet", func() {
		It("should error when creating incomplete fleet", func() {
			out, err := harness.CLIWithStdin(incompleteFleetYaml, "apply", "-f", "-")
			Expect(err).To(HaveOccurred())
			Expect(out).To(ContainSubstring("fleet: failed to apply"))
		})

		It("should work for a complete fleet", func() {
			// make sure it doesn't exist
			_, _ = harness.CLI("delete", "fleet/e2e-test-fleet")

			out, err := harness.CLIWithStdin(completeFleetYaml, "apply", "-f", "-")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring("201 Created"))

			// Applying a 2nd time it should also work, the fleet is just updated
			out, err = harness.CLIWithStdin(completeFleetYaml, "apply", "-f", "-")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring("200 OK"))
		})
	})

	Context("console", func() {
		It("should let you connect to a device", func() {
			deviceID := harness.StartVMAndEnroll()
			logrus.Infof("Attempting console connect command to device %s", deviceID)
			stdin, stdoutReader, err := harness.RunInteractiveCLI("console", "device/"+deviceID)
			Expect(err).ToNot(HaveOccurred())

			stdout := BufferReader(stdoutReader)

			send := func(cmd string) {
				_, err := stdin.Write([]byte(cmd + "\n"))
				Expect(err).ToNot(HaveOccurred())
			}

			// we don't have a virtual pty, so we need to make sure the console
			// will print a \n so stdout is flushed to us
			Eventually(stdout, TIMEOUT, POLLING).Should(Say(".*Connecting to .*\n"))
			Eventually(stdout, TIMEOUT, POLLING).Should(Say(".*to exit console.*\n"))

			logrus.Infof("Waiting for root prompt on device %s console", deviceID)
			send("")
			Eventually(stdout, TIMEOUT, POLLING).Should(Say(".*root@.*#"))

			logrus.Infof("Waiting fo ls output  on device %s console", deviceID)
			send("ls")

			Eventually(stdout, TIMEOUT, POLLING).Should(Say(".*bin"))

			logrus.Infof("Sending exit to the remote bash on device %s console", deviceID)
			send("exit")

			stdin.Close()

			// Make sure that there is no panic output from the console client
			Consistently(stdout, "2s").ShouldNot(Say(".*panic:"))
			stdout.Close()

		})

	})

	Context("certificate generation per user", func() {
		It("should have worked, and we can have a certificate", func() {
			out, err := harness.CLI("certificate", "request", "-n", randString(5))
			Expect(err).ToNot(HaveOccurred())
			Expect(out).To(ContainSubstring("certificate is ready"))
		})
	})

	Context("list devices", func() {
		It("Should let you list devices", func() {
			out, err := harness.CLI("get", "devices")
			Expect(err).ToNot(HaveOccurred())
			Expect(out).To(ContainSubstring("Fleet/default"))
		})
	})

	Context("list fleets", func() {
		It("Should let you list fleets", func() {
			out, err := harness.CLI("get", "fleets")
			Expect(err).ToNot(HaveOccurred())
			Expect(out).To(ContainSubstring("e2e-test-fleet"))
		})
	})
})

func randString(n int) string {
	const alphanum = "abcdefghijklmnopqrstuvwxyz"
	var bytes = make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		Expect(err).ToNot(HaveOccurred())
		return ""
	}
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	return string(bytes)
}

const completeFleetYaml = `
apiVersion: v1alpha1
kind: Fleet
metadata:
    name: e2e-test-fleet
spec:
    selector:
        matchLabels:
            fleet: default
    template:
        spec:
            os:
                image: quay.io/redhat/rhde:9.2
`

const incompleteFleetYaml = `
apiVersion: v1alpha1
kind: Fleet
metadata:
    name: e2e-test-fleet
spec:
    selector:
        matchLabels:
            fleet: default
`
