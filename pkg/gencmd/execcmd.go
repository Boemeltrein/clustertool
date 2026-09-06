package gencmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/trueforge-org/clustertool/pkg/helper"
	"github.com/trueforge-org/clustertool/pkg/nodestatus"
	fthelper "github.com/trueforge-org/forgetool/v4/pkg/helper"
)

// ExecCmd retries only the transient bootstrap-not-ready response.
func ExecCmd(cmd string) error {
	args := strings.Split(cmd, " ")
	deadline := time.Now().Add(2 * time.Minute)
	for {
		out, err := fthelper.RunCommand(args, false)
		if err == nil {
			return nil
		}
		if !strings.Contains(cmd, " bootstrap ") || !strings.Contains(string(out), "bootstrap is not available yet") {
			return fmt.Errorf("Talos command failed: %w: %s", err, strings.TrimSpace(string(out)))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("bootstrap is still unavailable: Talos may still be installing or installation may have failed; inspect talosctl dmesg and get disks, then check provisioning.diskSelector.match: %w: %s", err, strings.TrimSpace(string(out)))
		}
		time.Sleep(5 * time.Second)
	}
}

func ExecCmds(taloscmds []string, healthcheck bool) error {
	log.Info().Msg("Regenerating config prior to commands...")
	if err := GenConfig([]string{}); err != nil {
		return err
	}
	if len(taloscmds) == 0 {
		return nil
	}
	var todocmds []string
	var healthcmd string
	skipped := false
	if healthcheck {
		log.Info().Msg("Pre-Run Healthchecks...")

		for _, command := range taloscmds {

			node := helper.ExtractNode(command)
			log.Info().Msgf("checking node availability:  %v", node)
			err := nodestatus.CheckHealth(node, "", false)
			if err != nil {
				log.Info().Msgf("node seems not to be runnign correctly and cannot be used %v", node)
				log.Info().Msgf("node This will also make it impossible to poll total-cluster-health as well... %v", node)
				if !fthelper.GetYesOrNo("Do you want to continue without this node? (yes/no) [y/n]: ", false) {
					log.Info().Msg("Exiting...")
					return fmt.Errorf("Talos operation stopped")
				} else {
					skipped = true
					continue
				}
			}
			todocmds = append(todocmds, command)
		}
		if skipped {
			log.Info().Msg("skipping cluster health check due to unhealthy nodes being ignored...")
		} else {
			if fthelper.GetYesOrNo("Do you want to check the health of the cluster? (yes/no) [y/n]: ", false) {
				log.Info().Msg("Checking if cluster is healthy...")
				healthcmds := GenPlain("health", helper.TalEnv["VIP_IP"], []string{})
				if len(healthcmds) > 0 {
					healthcmd = healthcmds[0]
					if err := ExecCmd(healthcmd); err != nil {
						return err
					}
				}
			} else {
				skipped = true
			}
		}
	} else {
		todocmds = taloscmds
	}

	log.Info().Msg("Executing Cmds...")
	if len(todocmds) == 0 {
		return fmt.Errorf("no healthy configured node available; operation was not performed")
	}
	for _, command := range todocmds {
		node := helper.ExtractNode(command)
		log.Info().Msgf("Executing commands on node:  %v", node)
		argslice := strings.Split(string(command), " ")
		// log.Info().Msg("test", strings.Join(argslice, " "))
		log.Debug().Msgf("running command: %s", command)
		out, err := fthelper.RunCommand(argslice, false)
		if err != nil {
			if strings.Contains(command, " apply-config ") && strings.Contains(string(out), "certificate signed by unknown authority") {
				if err := nodestatus.CheckHealth(node, "maintenance", false); err != nil {
					return err
				}
				argslice = append(argslice, "--insecure")
				log.Debug().Msgf("Re-Running command using insecure flag: %s", command)
				_, err2 := fthelper.RunCommand(argslice, false)
				if err2 != nil {
					return fmt.Errorf("maintenance apply failed: %w", err2)
				}
			} else {
				return fmt.Errorf("Talos operation failed: %w: %s", err, strings.TrimSpace(string(out)))
			}

		}
		time.Sleep(15 * time.Second)

		if healthcheck {
			log.Info().Msgf("checking if node is back online:  %v", node)
			err := nodestatus.CheckHealth(node, "", false)
			if err != nil {
				log.Info().Msgf("node seems not to be running correctly... %v", node)
				if !fthelper.GetYesOrNo("Are you sure you want to continue applying this to other nodes? (yes/no) [y/n]: ", false) {
					log.Info().Msg("Exiting...")
					return fmt.Errorf("Talos operation stopped")
				}
			}
		}
	}

	if healthcheck && !skipped && healthcmd != "" && !strings.Contains(taloscmds[0], "upgrade") {
		log.Info().Msg("Checking if cluster is healthy after commands...")
		if err := ExecCmd(healthcmd); err != nil {
			return err
		}
	}
	return nil
}
