package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"go.uber.org/zap"

	"github.com/orinameh/aegis/internal/config"
	"github.com/orinameh/aegis/internal/guard"
)

// Pruner handles Docker cleanup operations
type Pruner struct {
	client *client.Client
	logger *zap.Logger
	guard  *guard.Guard
}

// NewPruner creates a new Docker pruner instance
func NewPruner(logger *zap.Logger, guard *guard.Guard) (*Pruner, error) {
	cli, err := client.New(
		client.FromEnv,
		client.WithAPIVersionFromEnv(),
		client.WithTimeout(30*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := cli.Ping(ctx, client.PingOptions{}); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("failed to ping Docker daemon: %w", err)
	}

	logger.Info("Docker client initialized",
		zap.String("api_version", cli.ClientVersion()),
	)

	return &Pruner{
		client: cli,
		logger: logger,
		guard:  guard,
	}, nil
}

// Close closes the Docker client connection
func (p *Pruner) Close() error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}

// Prune executes the Docker cleanup operations
func (p *Pruner) Prune(ctx context.Context, cfg *config.DockerConfig) error {
	p.logger.Info("starting Docker pruner")

	var totalSpaceReclaimed uint64
	var results []string

	// Prune stopped containers
	if cfg.PruneStopped {
		space, count, err := p.pruneContainers(ctx)
		if err != nil {
			p.logger.Error("failed to prune containers", zap.Error(err))
			return err
		}
		totalSpaceReclaimed += space
		results = append(results, fmt.Sprintf("pruned %d containers", count))
	}

	// Prune dangling images
	if cfg.PruneDangling {
		space, count, err := p.pruneImages(ctx)
		if err != nil {
			p.logger.Error("failed to prune images", zap.Error(err))
			return err
		}
		totalSpaceReclaimed += space
		results = append(results, fmt.Sprintf("pruned %d dangling images", count))
	}

	// Prune build cache
	if cfg.PruneBuildCache {
		space, count, err := p.pruneBuildCache(ctx)
		if err != nil {
			p.logger.Error("failed to prune build cache", zap.Error(err))
			return err
		}
		totalSpaceReclaimed += space
		results = append(results, fmt.Sprintf("pruned %d cache entries", count))
	}

	// Prune networks
	if cfg.PruneNetworks {
		space, count, err := p.pruneNetworks(ctx)
		if err != nil {
			p.logger.Error("failed to prune networks", zap.Error(err))
			return err
		}
		totalSpaceReclaimed += space
		results = append(results, fmt.Sprintf("pruned %d networks", count))
	}

	// Prune volumes
	if cfg.PruneVolumes {
		space, count, err := p.pruneVolumes(ctx)
		if err != nil {
			p.logger.Error("failed to prune volumes", zap.Error(err))
			return err
		}
		totalSpaceReclaimed += space
		results = append(results, fmt.Sprintf("pruned %d volumes", count))
	}

	p.logger.Info("Docker pruner completed",
		zap.Strings("results", results),
		zap.Uint64("total_space_reclaimed_bytes", totalSpaceReclaimed),
		zap.String("total_space_reclaimed_human", humanizeBytes(totalSpaceReclaimed)),
	)

	return nil
}

// pruneContainers removes stopped containers with protection checks
func (p *Pruner) pruneContainers(ctx context.Context) (uint64, int, error) {
	p.logger.Info("pruning stopped containers")

	// List stopped containers. Size:true asks the daemon to include each
	// container's disk usage in the response so we don't need a follow-up
	// ContainerInspect call per container.
	listResult, err := p.client.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Size:    true,
		Filters: client.Filters{}.Add("status", "exited"),
	})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list containers: %w", err)
	}

	var totalSpaceReclaimed uint64
	var prunedCount int

	for _, ctr := range listResult.Items {
		// Get container name (remove leading slash)
		containerName := "unknown"
		if len(ctr.Names) > 0 {
			containerName = strings.TrimPrefix(ctr.Names[0], "/")
		}

		containerSize := ctr.SizeRw

		resource := &guard.Resource{
			Type:      guard.ResourceContainer,
			Name:      containerName,
			Namespace: "docker",
			Labels:    ctr.Labels,
			Metadata: map[string]any{
				"container_id": ctr.ID,
				"image":        ctr.Image,
				"created":      ctr.Created,
				"status":       ctr.Status,
				"size":         containerSize,
				"image_id":     ctr.ImageID,
			},
		}

		err := p.guard.CheckAndExecute(ctx, resource, "delete", func() error {
			// Remove the container
			if _, err := p.client.ContainerRemove(ctx, ctr.ID, client.ContainerRemoveOptions{
				RemoveVolumes: false,
				Force:         false,
			}); err != nil {
				return fmt.Errorf("failed to remove container %s: %w", containerName, err)
			}

			if containerSize > 0 {
				totalSpaceReclaimed += uint64(containerSize)
			}
			prunedCount++

			p.logger.Debug("container pruned",
				zap.String("container", containerName),
				zap.Int64("size_bytes", containerSize),
			)
			return nil
		})

		if err != nil {
			if !isProtectionDenial(err) {
				p.logger.Error("failed to delete container",
					zap.String("container", containerName),
					zap.Error(err),
				)
			} else {
				p.logger.Debug("container protected, skipping",
					zap.String("container", containerName),
					zap.Error(err),
				)
			}
		}
	}

	p.logger.Info("container pruning completed",
		zap.Int("pruned", prunedCount),
		zap.Uint64("space_reclaimed_bytes", totalSpaceReclaimed),
	)

	return totalSpaceReclaimed, prunedCount, nil
}

// pruneImages removes dangling images with protection checks
func (p *Pruner) pruneImages(ctx context.Context) (uint64, int, error) {
	p.logger.Info("pruning dangling images")

	listResult, err := p.client.ImageList(ctx, client.ImageListOptions{
		Filters: client.Filters{}.Add("dangling", "true"),
	})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list images: %w", err)
	}

	var totalSpaceReclaimed uint64
	var prunedCount int

	for _, img := range listResult.Items {
		imageName := "<none>"
		if len(img.RepoTags) > 0 && img.RepoTags[0] != "<none>:<none>" {
			imageName = img.RepoTags[0]
		}

		resource := &guard.Resource{
			Type:      guard.ResourceImage,
			Name:      imageName,
			Namespace: "docker",
			Labels:    map[string]string{},
			Metadata: map[string]any{
				"image_id":     img.ID,
				"size":         img.Size,
				"created":      img.Created,
				"repo_tags":    img.RepoTags,
				"repo_digests": img.RepoDigests,
			},
		}

		err := p.guard.CheckAndExecute(ctx, resource, "delete", func() error {
			// Remove the image
			if _, err := p.client.ImageRemove(ctx, img.ID, client.ImageRemoveOptions{
				Force:         false,
				PruneChildren: false,
			}); err != nil {
				return fmt.Errorf("failed to remove image %s: %w", imageName, err)
			}
			totalSpaceReclaimed += uint64(img.Size)
			prunedCount++
			p.logger.Debug("image pruned",
				zap.String("image", imageName),
				zap.Int64("size_bytes", img.Size),
			)
			return nil
		})

		if err != nil {
			if !isProtectionDenial(err) {
				p.logger.Error("failed to delete image",
					zap.String("image", imageName),
					zap.Error(err),
				)
			} else {
				p.logger.Debug("image protected, skipping",
					zap.String("image", imageName),
					zap.Error(err),
				)
			}
		}
	}

	p.logger.Info("image pruning completed",
		zap.Int("pruned", prunedCount),
		zap.Uint64("space_reclaimed_bytes", totalSpaceReclaimed),
	)

	return totalSpaceReclaimed, prunedCount, nil
}

// pruneBuildCache removes build cache with protection checks.
//
// Note: the client no longer exposes a standalone BuildCacheList method
// (that was removed from the v0.5.x API surface — cache usage is now
// reported via DiskUsage). We can't cheaply pre-count entries before
// pruning, so we protection-check the operation as a whole and report
// counts/space from BuildCachePrune's own result.
func (p *Pruner) pruneBuildCache(ctx context.Context) (uint64, int, error) {
	p.logger.Info("pruning build cache")

	resource := &guard.Resource{
		Type:      guard.ResourceCache,
		Name:      "docker-build-cache",
		Namespace: "docker",
		Labels:    map[string]string{},
		Metadata:  map[string]any{},
	}

	var totalSpaceReclaimed uint64
	var prunedCount int

	err := p.guard.CheckAndExecute(ctx, resource, "prune", func() error {
		pruneResult, err := p.client.BuildCachePrune(ctx, client.BuildCachePruneOptions{
			All: true,
		})
		if err != nil {
			return fmt.Errorf("failed to prune build cache: %w", err)
		}
		totalSpaceReclaimed = pruneResult.Report.SpaceReclaimed
		prunedCount = len(pruneResult.Report.CachesDeleted)

		p.logger.Debug("build cache pruned",
			zap.Int("cache_entries_removed", prunedCount),
			zap.Uint64("space_reclaimed_bytes", totalSpaceReclaimed),
		)
		return nil
	})

	if err != nil {
		if !isProtectionDenial(err) {
			return 0, 0, fmt.Errorf("failed to prune build cache: %w", err)
		}
		p.logger.Debug("build cache protected, skipping", zap.Error(err))
		return 0, 0, nil
	}

	p.logger.Info("build cache pruning completed",
		zap.Int("pruned", prunedCount),
		zap.Uint64("space_reclaimed_bytes", totalSpaceReclaimed),
	)

	return totalSpaceReclaimed, prunedCount, nil
}

// pruneNetworks removes unused networks
func (p *Pruner) pruneNetworks(ctx context.Context) (uint64, int, error) {
	p.logger.Info("pruning unused networks")

	listResult, err := p.client.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list networks: %w", err)
	}

	var prunedCount int
	for _, network := range listResult.Items {
		// Skip default networks
		if network.Name == "bridge" || network.Name == "host" || network.Name == "none" {
			continue
		}

		// BUG FIX: network.Summary (what NetworkList returns) no longer
		// carries a Containers field — that data was split out into
		// network.Inspect only (moby/moby#50878). Checking `.Containers`
		// on a list item either fails to compile, or — if it did compile
		// against some intermediate type — would always read as empty,
		// meaning this check silently never fired and the pruner would
		// delete networks that still had containers attached. We now
		// inspect each candidate network individually to get its real
		// attached-endpoint count before deciding whether it's unused.
		inspectResult, err := p.client.NetworkInspect(ctx, network.ID, client.NetworkInspectOptions{})
		if err != nil {
			p.logger.Warn("failed to inspect network, skipping to be safe",
				zap.String("network", network.Name),
				zap.Error(err),
			)
			continue
		}
		if len(inspectResult.Network.Containers) > 0 {
			continue
		}

		resource := &guard.Resource{
			Type:      guard.ResourceNetwork,
			Name:      network.Name,
			Namespace: "docker",
			Labels:    network.Labels,
			Metadata: map[string]any{
				"network_id": network.ID,
				"scope":      network.Scope,
				"driver":     network.Driver,
				"internal":   network.Internal,
				"attachable": network.Attachable,
			},
		}

		// NOTE: `err` was already declared above by the NetworkInspect call
		// in this same loop-body scope, so this must be a plain assignment
		// (`=`), not `:=` — `:=` here would fail to compile with "no new
		// variables on left side of :=" since err is the only identifier
		// on the left and it isn't new.
		err = p.guard.CheckAndExecute(ctx, resource, "delete", func() error {
			if _, err := p.client.NetworkRemove(ctx, network.ID, client.NetworkRemoveOptions{}); err != nil {
				return fmt.Errorf("failed to remove network %s: %w", network.Name, err)
			}
			prunedCount++
			p.logger.Debug("network pruned",
				zap.String("network", network.Name),
			)
			return nil
		})

		if err != nil {
			if !isProtectionDenial(err) {
				p.logger.Error("failed to delete network",
					zap.String("network", network.Name),
					zap.Error(err),
				)
			} else {
				p.logger.Debug("network protected, skipping",
					zap.String("network", network.Name),
					zap.Error(err),
				)
			}
		}
	}

	p.logger.Info("network pruning completed",
		zap.Int("pruned", prunedCount),
	)

	return 0, prunedCount, nil
}

// pruneVolumes removes unused volumes
func (p *Pruner) pruneVolumes(ctx context.Context) (uint64, int, error) {
	p.logger.Info("pruning unused volumes")

	listResult, err := p.client.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list volumes: %w", err)
	}

	var totalSpaceReclaimed uint64
	var prunedCount int

	for _, vol := range listResult.Items {
		// BUG FIX: the original condition was
		//   if volume.UsageData != nil && volume.UsageData.RefCount > 0 { continue }
		// which does NOT skip when UsageData is nil (the && short-circuits
		// the whole condition to false), so execution fell through into
		// `volume.UsageData.Size` below and panicked with a nil pointer
		// dereference whenever usage data wasn't populated. We now treat
		// "no usage data" as "unknown, don't touch it" and guard every
		// subsequent access.
		if vol.UsageData == nil {
			p.logger.Debug("volume has no usage data, skipping",
				zap.String("volume", vol.Name),
			)
			continue
		}
		if vol.UsageData.RefCount > 0 {
			continue
		}

		volSize := vol.UsageData.Size

		resource := &guard.Resource{
			Type:      guard.ResourceVolume,
			Name:      vol.Name,
			Namespace: "docker",
			Labels:    vol.Labels,
			Metadata: map[string]any{
				"driver":     vol.Driver,
				"scope":      vol.Scope,
				"mountpoint": vol.Mountpoint,
				"size":       volSize,
				"ref_count":  vol.UsageData.RefCount,
			},
		}

		err := p.guard.CheckAndExecute(ctx, resource, "delete", func() error {
			if _, err := p.client.VolumeRemove(ctx, vol.Name, client.VolumeRemoveOptions{Force: false}); err != nil {
				return fmt.Errorf("failed to remove volume %s: %w", vol.Name, err)
			}
			totalSpaceReclaimed += uint64(volSize)
			prunedCount++
			p.logger.Debug("volume pruned",
				zap.String("volume", vol.Name),
				zap.Int64("size_bytes", volSize),
			)
			return nil
		})

		if err != nil {
			if !isProtectionDenial(err) {
				p.logger.Error("failed to delete volume",
					zap.String("volume", vol.Name),
					zap.Error(err),
				)
			} else {
				p.logger.Debug("volume protected, skipping",
					zap.String("volume", vol.Name),
					zap.Error(err),
				)
			}
		}
	}

	p.logger.Info("volume pruning completed",
		zap.Int("pruned", prunedCount),
		zap.Uint64("space_reclaimed_bytes", totalSpaceReclaimed),
	)

	return totalSpaceReclaimed, prunedCount, nil
}

// isProtectionDenial checks if an error is a protection denial
func isProtectionDenial(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "deletion not approved") ||
		strings.Contains(msg, "not allowed") ||
		strings.Contains(msg, "critically protected") ||
		strings.Contains(msg, "strictly protected")
}

// humanizeBytes converts bytes to human-readable format
func humanizeBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
