// Copyright 2022 Nokia
// Licensed under the BSD 3-Clause License.
// SPDX-License-Identifier: BSD-3-Clause

package xrd

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/srl-labs/containerlab/netconf"
	"github.com/srl-labs/containerlab/nodes"
	"github.com/srl-labs/containerlab/types"
	"github.com/srl-labs/containerlab/utils"
)

var (
	kindnames = []string{"xrd", "cisco_xrd"}
	xrdEnv    = map[string]string{
		"XR_FIRST_BOOT_CONFIG": "/etc/xrd/first-boot.cfg",
		"XR_MGMT_INTERFACES":   "linux:eth0,xr_name=Mg0/RP0/CPU0/0,chksum,snoop_v4,snoop_v6",
	}

	//go:embed xrd.cfg
	cfgTemplate string
)

const (
	scrapliPlatformName = "cisco_iosxr"
	defaultUser         = "clab"
	defaultPassword     = "clab@123"
)

// Register registers the node in the global Node map.
func Register() {
	nodes.Register(kindnames, func() nodes.Node {
		return new(xrd)
	})
	err := nodes.SetDefaultCredentials(kindnames, defaultUser, defaultPassword)
	if err != nil {
		log.Error(err)
	}
}

type xrd struct {
	nodes.DefaultNode
}

func (n *xrd) Init(cfg *types.NodeConfig, opts ...nodes.NodeOption) error {
	// Init DefaultNode
	n.DefaultNode = *nodes.NewDefaultNode(n)

	n.Cfg = cfg
	for _, o := range opts {
		o(n)
	}

	var interfaceEnvContent string
	for i := 1; i <= 90; i++ {
		interfaceEnvContent += fmt.Sprintf("linux:eth%d,xr_name=Gi0/0/0/%d;", i, i-1)
	}
	interfaceEnv := map[string]string{"XR_INTERFACES": interfaceEnvContent}

	n.Cfg.Env = utils.MergeStringMaps(xrdEnv, interfaceEnv, n.Cfg.Env)

	n.Cfg.Binds = append(n.Cfg.Binds,
		// mount first-boot config file
		fmt.Sprint(filepath.Join(n.Cfg.LabDir, "first-boot.cfg"), ":/etc/xrd/first-boot.cfg"),
		// persist data by mounting /xr-storage
		fmt.Sprint(filepath.Join(n.Cfg.LabDir, "xr-storage"), ":/xr-storage"),
	)

	return nil
}

func (n *xrd) PreDeploy(ctx context.Context, _, _, _ string) error {
	utils.CreateDirectory(n.Cfg.LabDir, 0777)
	return n.createXRDFiles(ctx)
}

func (n *xrd) SaveConfig(_ context.Context) error {
	err := netconf.SaveConfig(n.Cfg.LongName,
		defaultUser,
		defaultPassword,
		scrapliPlatformName,
	)
	if err != nil {
		return err
	}

	log.Infof("saved %s running configuration to startup configuration file\n", n.Cfg.ShortName)
	return nil
}

func (n *xrd) createXRDFiles(_ context.Context) error {
	nodeCfg := n.Config()
	// generate xr-storage directory
	utils.CreateDirectory(filepath.Join(n.Cfg.LabDir, "xr-storage"), 0777)
	// generate first-boot config
	cfg := filepath.Join(n.Cfg.LabDir, "first-boot.cfg")
	nodeCfg.ResStartupConfig = cfg

	// set mgmt IPv4/IPv6 gateway as it is already known by now
	// since the container network has been created before we launch nodes
	// and mgmt gateway can be used in xrd.Cfg template to configure default route for mgmt
	nodeCfg.MgmtIPv4Gateway = n.Runtime.Mgmt().IPv4Gw
	nodeCfg.MgmtIPv6Gateway = n.Runtime.Mgmt().IPv6Gw

	// use startup config file provided by a user
	if nodeCfg.StartupConfig != "" {
		c, err := os.ReadFile(nodeCfg.StartupConfig)
		if err != nil {
			return err
		}
		cfgTemplate = string(c)
	}

	err := n.GenerateConfig(nodeCfg.ResStartupConfig, cfgTemplate)
	if err != nil {
		return err
	}

	return err
}
