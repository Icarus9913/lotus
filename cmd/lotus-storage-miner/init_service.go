package main

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"strings"
)

func checkApiInfo(ctx context.Context, ai string) (string, error) {
	ai = strings.TrimPrefix(strings.TrimSpace(ai), "MINER_API_INFO=")
	info := cliutil.ParseApiInfo(ai)
	addr, err := info.DialArgs()
	if err != nil {
		return "", xerrors.Errorf("could not get DialArgs: %w", err)
	}

	log.Infof("Checking api version of %s", addr)

	api, closer, err := client.NewStorageMinerRPC(ctx, addr, info.AuthHeader())
	if err != nil {
		return "", err
	}
	defer closer()

	v, err := api.Version(ctx)
	if err != nil {
		return "", xerrors.Errorf("checking version: %w", err)
	}

	if !v.APIVersion.EqMajorMinor(lapi.MinerAPIVersion) {
		return "", xerrors.Errorf("remote service API version didn't match (expected %s, remote %s)", lapi.MinerAPIVersion, v.APIVersion)
	}

	return ai, nil
}

var initServiceCmd = &cli.Command{
	Name:  "service",
	Usage: "Initialize a lotus miner sub-service",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "config",
			Usage:    "config file (config.toml)",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "storage-config",
			Usage:    "storage paths config (storage.json)",
			Required: true,
		},
		&cli.BoolFlag{
			Name:  "nosync",
			Usage: "don't check full-node sync status",
		},

		&cli.BoolFlag{
			Name:  "enable-market",
			Usage: "enable market module",
		},

		&cli.StringFlag{
			Name:  "api-sealer",
			Usage: "sealer API info (lotus-miner auth api-info --perm=admin)",
		},
		&cli.StringFlag{
			Name:  "api-sector-index",
			Usage: "sector Index API info (lotus-miner auth api-info --perm=admin)",
		},
	},
	ArgsUsage: "[backupFile]",
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)
		log.Info("Initializing lotus miner service")

		if !cctx.Bool("enable-market") {
			return xerrors.Errorf("at least one module must be enabled")
		}

		if !cctx.IsSet("api-sealer") {
			return xerrors.Errorf("--api-sealer is required without the sealer module enabled")
		}
		if !cctx.IsSet("api-sector-index") {
			return xerrors.Errorf("--api-sector-index is required without the sector storage module enabled")
		}

		if err := restore(ctx, cctx, func(cfg *config.StorageMiner) error {
			cfg.Subsystems.EnableStorageMarket = cctx.Bool("enable-market")
			cfg.Subsystems.EnableMining = false
			cfg.Subsystems.EnableSealing = false
			cfg.Subsystems.EnableSectorStorage = false

			if !cfg.Subsystems.EnableSealing {
				ai, err := checkApiInfo(ctx, cctx.String("api-sealer"))
				if err != nil {
					return xerrors.Errorf("checking sealer API: %w", err)
				}
				cfg.Subsystems.SealerApiInfo = ai
			}

			if !cfg.Subsystems.EnableSectorStorage {
				ai, err := checkApiInfo(ctx, cctx.String("api-sector-index"))
				if err != nil {
					return xerrors.Errorf("checking sector index API: %w", err)
				}
				cfg.Subsystems.SectorIndexApiInfo = ai
			}

			return nil
		}, func(api lapi.FullNode, maddr address.Address, peerid peer.ID, mi miner.MinerInfo) error {
			if cctx.Bool("enable-market") {
				log.Info("Configuring miner actor")

				if err := configureStorageMiner(ctx, api, maddr, peerid, big.Zero()); err != nil {
					return err
				}
			}

			return nil
		}); err != nil {
			return err
		}

		return nil
	},
}
