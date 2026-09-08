package initfiles

import (
	"bufio"
	"net"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/rs/zerolog/log"
	"github.com/trueforge-org/clustertool/pkg/helper"
	"github.com/trueforge-org/clustertool/pkg/talosconfig"
	fthelper "github.com/trueforge-org/forgetool/v4/pkg/helper"
)

func LoadTalEnv(noFail bool) error {
	// Check if clusterenv.yaml file exists
	if _, err := os.Stat(helper.ClusterPath + "/clusterenv.yaml"); err == nil {
		// Load environment variables from clusterenv.yaml
		// Reload only source values. Previously derived IP variables must not
		// become inputs on the next load during apply/bootstrap.
		sourceEnv := make(map[string]string)
		err := fthelper.LoadEnvFromFile(helper.ClusterPath+"/clusterenv.yaml", sourceEnv)
		if err != nil {
			log.Info().Msgf("Error loading environment from clusterenv.yaml: %v\n", err)
			os.Exit(1)
		}
		helper.TalEnv = sourceEnv
	} else if os.IsNotExist(err) {
		// If the file doesn't exist, check noFail to determine next steps
		if noFail {
			log.Debug().Msg("clusterenv.yaml file not found, but skipping due to noFail being true.")
			return nil // Skip execution without error
		} else {
			log.Fatal().Msg("clusterenv.yaml file not found, exiting...")
			os.Exit(1) // Exit with error code 1
		}
	} else {
		log.Info().Msgf("Error checking clusterenv.yaml file: %v\n", err)
		os.Exit(1)
	}

	// If file exists, continue with processing
	clusterName()
	checkQuotedNumbersInFile()
	clusterEnvtoEnv()
	log.Info().Msgf("ClusterEnv loaded successfully\n")
	return nil
}

// Function to check if all numbers after ':' in a file are unquoted integers or floats
func checkQuotedNumbersInFile() (bool, error) {

	filePath := helper.ClusterPath + "/clusterenv.yaml"
	// Regular expression to find patterns like ': number' where number can be an int or float
	re := regexp.MustCompile(`:\s*(.+)`) // Matches anything after ': '

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		log.Error().Msgf("Failed to open file: %s \nError: %s", filePath, err)
		os.Exit(1)
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip lines that start with a number
		trimmedLine := strings.TrimSpace(line)
		if len(trimmedLine) > 0 && unicode.IsDigit(rune(trimmedLine[0])) {
			continue
		}

		// Find matches for entries in each line
		matches := re.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue // Skip lines without a colon and value
		}

		// Get the value after the colon
		value := strings.TrimSpace(matches[1])

		// Check if the value is a valid number (int or float with a single dot)
		isValidNumber := regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`).MatchString(value)

		// If it's a valid number, log an error
		if isValidNumber {
			log.Error().Msgf("Unquoted number found %s line: %s", filePath, line)
			os.Exit(1)
			return false, nil
		}
	}

	if err := scanner.Err(); err != nil {
		log.Error().Msgf("Error scanning the file: %s", err)
		os.Exit(1)
		return false, err
	}

	return true, nil
}

func clusterName() {
	helper.TalEnv["CLUSTERNAME"] = helper.ClusterName
}

func clusterEnvtoEnv() {
	// Export source values without synthesizing address variants.
	for key, value := range helper.TalEnv {
		os.Setenv(key, value)
	}
}
func CheckEnvVariables() {
	LoadTalEnv(false)
	requiredKeys := []string{
		"VIP",
		"HEADLAMP_IP",
		"GATEWAY",
		"METALLB_RANGE",
		"PODNET",
		"SVCNET",
		"DOMAIN_0",
		"DOMAIN_0_EMAIL",
		"DOMAIN_0_CLOUDFLARE_TOKEN",
	}
	for _, key := range requiredKeys {
		if helper.TalEnv[key] == "" {
			log.Info().Msgf("%s cannot be empty\n", key)
			os.Exit(1)
		}
	}

	for _, key := range []string{"VIP", "GATEWAY"} {
		if net.ParseIP(helper.TalEnv[key]) == nil {
			log.Error().Msgf("%s must be an IP address without a subnet prefix", key)
			os.Exit(1)
		}
	}
	inv, err := talosconfig.LoadInventory()
	if err != nil {
		log.Error().Err(err).Msg("Invalid clustertool.yaml")
		os.Exit(1)
	}
	for _, node := range inv.Nodes {
		if node.Address == helper.TalEnv["VIP"] || node.Address == helper.TalEnv["GATEWAY"] {
			log.Error().Msgf("Node %s management address overlaps VIP or gateway", node.Name)
			os.Exit(1)
		}
		inRange, err := fthelper.IPInRange(node.Address, helper.TalEnv["METALLB_RANGE"])
		if err != nil || inRange {
			log.Error().Err(err).Msgf("Node %s management address conflicts with METALLB_RANGE", node.Name)
			os.Exit(1)
		}
		for _, network := range []string{"PODNET", "SVCNET"} {
			_, prefix, err := net.ParseCIDR(helper.TalEnv[network])
			if err != nil || prefix.Contains(net.ParseIP(node.Address)) {
				log.Error().Err(err).Msgf("Node %s management address conflicts with %s", node.Name, network)
				os.Exit(1)
			}
		}
	}
	// Retain the shared-network checks using the inventory bootstrap address.
	vip := helper.TalEnv["VIP"]
	bootstrapIP := inv.Bootstrap().Address
	bootstrapCIDR := bootstrapIP + "/32"
	if strings.Contains(bootstrapIP, ":") {
		bootstrapCIDR = bootstrapIP + "/128"
	}
	gateway := helper.TalEnv["GATEWAY"]

	// Check if bootstrap node matches GATEWAY or VIP
	if bootstrapIP == gateway || bootstrapIP == vip {
		log.Info().Msg("Cannot proceed, bootstrap node cannot match GATEWAY or VIP")
		os.Exit(1)
	}

	// Check if VIP matches any Node IPs
	if vip == bootstrapIP {
		log.Info().Msg("Cannot proceed, VIP cannot match any Node IPs")
		os.Exit(1)
	}

	// Check ranges against METALLB_RANGE
	inRange, err := fthelper.IPInRange(vip, helper.TalEnv["METALLB_RANGE"])
	if err != nil {
		log.Info().Msgf("Error checking VIP against METALLB_RANGE: %v\n", err)
		os.Exit(1)
	}
	if inRange {
		log.Info().Msg("Cannot proceed, VIP cannot be in the METALLB_RANGE")
		os.Exit(1)
	}

	inRange, err = fthelper.IPInRange(bootstrapIP, helper.TalEnv["METALLB_RANGE"])
	if err != nil {
		log.Info().Msgf("Error checking bootstrap node against METALLB_RANGE: %v\n", err)
		os.Exit(1)
	}
	if inRange {
		log.Info().Msg("Cannot proceed, bootstrap node cannot be in the METALLB_RANGE")
		os.Exit(1)
	}

	inRange, err = fthelper.IPInRange(gateway, helper.TalEnv["METALLB_RANGE"])
	if err != nil {
		log.Info().Msgf("Error checking GATEWAY against METALLB_RANGE: %v\n", err)
		os.Exit(1)
	}
	if inRange {
		log.Info().Msg("Cannot proceed, GATEWAY cannot be in the METALLB_RANGE")
		os.Exit(1)
	}

	// Check HEADLAMP_IP against METALLB_RANGE
	if helper.TalEnv["HEADLAMP_IP"] != "" {
		inRange, err = fthelper.IPInRange(helper.TalEnv["HEADLAMP_IP"], helper.TalEnv["METALLB_RANGE"])
		if err != nil {
			log.Info().Msgf("Error checking HEADLAMP_IP against METALLB_RANGE: %v\n", err)
			os.Exit(1)
		}
		if !inRange {
			log.Info().Msg("Cannot proceed, HEADLAMP_IP must be in the METALLB_RANGE")
			os.Exit(1)
		}
	}

	// Validate other CIDR/IP checks with new netmask support
	fthelper.ValidateIPorCIDRNotInCIDR(vip+"/32", helper.TalEnv["PODNET"], "VIP", "PODNET")
	fthelper.ValidateIPorCIDRNotInCIDR(bootstrapCIDR, helper.TalEnv["PODNET"], "bootstrap node", "PODNET")
	fthelper.ValidateIPorCIDRNotInCIDR(gateway+"/32", helper.TalEnv["PODNET"], "GATEWAY", "PODNET")
	fthelper.ValidateRangeNotInCIDR(helper.TalEnv["METALLB_RANGE"], helper.TalEnv["PODNET"], "METALLB_RANGE", "PODNET")

	fthelper.ValidateIPorCIDRNotInCIDR(vip+"/32", helper.TalEnv["SVCNET"], "VIP", "SVCNET")
	fthelper.ValidateIPorCIDRNotInCIDR(bootstrapCIDR, helper.TalEnv["SVCNET"], "bootstrap node", "SVCNET")
	fthelper.ValidateIPorCIDRNotInCIDR(gateway+"/32", helper.TalEnv["SVCNET"], "GATEWAY", "SVCNET")
	fthelper.ValidateRangeNotInCIDR(helper.TalEnv["METALLB_RANGE"], helper.TalEnv["SVCNET"], "METALLB_RANGE", "SVCNET")
}
