// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2023, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.

package pkg

import (
	"context"
	"fmt"
	"strings"

	"kraftkit.sh/config"
	"kraftkit.sh/internal/cli/kraft/utils"
	"kraftkit.sh/log"
	"kraftkit.sh/pack"
	"kraftkit.sh/packmanager"
	"kraftkit.sh/tui/processtree"
	"kraftkit.sh/unikraft/arch"
	"kraftkit.sh/unikraft/plat"
	"kraftkit.sh/unikraft/target"
)

type packagerCliKernel struct{}

// String implements fmt.Stringer.
func (p *packagerCliKernel) String() string {
	return "cli-kernel"
}

// Packagable implements packager.
func (p *packagerCliKernel) Packagable(ctx context.Context, opts *PkgOptions, args ...string) (bool, error) {
	if len(opts.Kernel) > 0 && len(opts.Platform) > 0 {
		if len(opts.Architecture) == 0 && strings.Contains(opts.Platform, "/") {
			opts.Platform, opts.Architecture, _ = strings.Cut(opts.Platform, "/")
		}
		return true, nil
	}

	if len(opts.Kernel) > 0 {
		log.G(ctx).Warn("--kernel flag set but must be used in conjunction with -m|--arch and/or -p|--plat")
	}

	return false, fmt.Errorf("cannot package without path to -k|-kernel, -m|--arch and -p|--plat")
}

// Pack implements packager.
func (p *packagerCliKernel) Pack(ctx context.Context, opts *PkgOptions, args ...string) ([]pack.Package, error) {
	var err error

	ac := arch.NewArchitectureFromOptions(
		arch.WithName(opts.Architecture),
	)
	pc := plat.NewPlatformFromOptions(
		plat.WithName(opts.Platform),
	)

	targ := target.NewTargetFromOptions(
		target.WithArchitecture(ac),
		target.WithPlatform(pc),
		target.WithKernel(opts.Kernel),
		target.WithCommand(opts.Args),
	)

	var cmds []string
	var envs []string
	if opts.Rootfs, cmds, envs, err = utils.BuildRootfs(ctx, opts.Workdir, opts.Rootfs, opts.Compress, targ.Architecture().String()); err != nil {
		return nil, fmt.Errorf("could not build rootfs: %w", err)
	}

	if len(opts.Args) == 0 && cmds != nil {
		opts.Args = cmds
	}

	if envs != nil {
		opts.Env = append(opts.Env, envs...)
	}

	var result []pack.Package
	norender := log.LoggerTypeFromString(config.G[config.KraftKit](ctx).Log.Type) != log.FANCY

	model, err := processtree.NewProcessTree(
		ctx,
		[]processtree.ProcessTreeOption{
			processtree.IsParallel(false),
			processtree.WithRenderer(norender),
		},

		processtree.NewProcessTreeItem(
			"packaging "+opts.Name+" ("+opts.Format+")",
			opts.Platform+"/"+opts.Architecture,
			func(ctx context.Context) error {
				popts := append(opts.packopts,
					packmanager.PackArgs(opts.Args...),
					packmanager.PackInitrd(opts.Rootfs),
					packmanager.PackKConfig(!opts.NoKConfig),
					packmanager.PackName(opts.Name),
					packmanager.PackOutput(opts.Output),
				)

				envs := opts.aggregateEnvs()
				if len(envs) > 0 {
					popts = append(popts, packmanager.PackWithEnvs(envs))
				} else if len(opts.Env) > 0 {
					popts = append(popts, packmanager.PackWithEnvs(opts.Env))
				}

				more, err := opts.pm.Pack(ctx, targ, popts...)
				if err != nil {
					return err
				}

				result = append(result, more...)

				return nil
			},
		),
	)
	if err != nil {
		return nil, err
	}

	if err := model.Start(); err != nil {
		return nil, err
	}

	return result, nil
}
