package docker

import (
	"context"
	"fmt"

	"github.com/moby/moby/client"
)

// ContainerSummary is a read-only snapshot of a Docker container.
type ContainerSummary struct {
	Name    string
	ID      string
	Image   string
	Status  string
	Created int64
	Size    int64
}

// ImageSummary is a read-only snapshot of a Docker image.
type ImageSummary struct {
	ID         string
	RepoTags   []string
	Size       int64
	Created    int64
	Containers int64
}

// VolumeSummary is a read-only snapshot of a Docker volume.
type VolumeSummary struct {
	Name     string
	Driver   string
	Scope    string
	Size     int64
	RefCount int64
}

// NetworkSummary is a read-only snapshot of a Docker network.
type NetworkSummary struct {
	Name       string
	ID         string
	Driver     string
	Scope      string
	Internal   bool
	Attachable bool
}

// Inventory is a read-only snapshot of all Docker resources.
type Inventory struct {
	Containers []ContainerSummary
	Images     []ImageSummary
	Volumes    []VolumeSummary
	Networks   []NetworkSummary
}

// List returns a read-only inventory of all Docker resources. It performs no
// mutations, so it is safe to run at any time.
func (p *Pruner) List(ctx context.Context) (*Inventory, error) {
	var inv Inventory
	var err error

	if inv.Containers, err = p.listContainers(ctx); err != nil {
		return nil, err
	}
	if inv.Images, err = p.listImages(ctx); err != nil {
		return nil, err
	}
	if inv.Volumes, err = p.listVolumes(ctx); err != nil {
		return nil, err
	}
	if inv.Networks, err = p.listNetworks(ctx); err != nil {
		return nil, err
	}

	return &inv, nil
}

func (p *Pruner) listContainers(ctx context.Context) ([]ContainerSummary, error) {
	result, err := p.client.ContainerList(ctx, client.ContainerListOptions{
		All:  true,
		Size: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	items := make([]ContainerSummary, 0, len(result.Items))
	for _, c := range result.Items {
		name := "unknown"
		if len(c.Names) > 0 {
			name = c.Names[0]
		}
		items = append(items, ContainerSummary{
			Name:    name,
			ID:      c.ID,
			Image:   c.Image,
			Status:  c.Status,
			Created: c.Created,
			Size:    c.SizeRw,
		})
	}
	return items, nil
}

func (p *Pruner) listImages(ctx context.Context) ([]ImageSummary, error) {
	result, err := p.client.ImageList(ctx, client.ImageListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	items := make([]ImageSummary, 0, len(result.Items))
	for _, img := range result.Items {
		items = append(items, ImageSummary{
			ID:         img.ID,
			RepoTags:   img.RepoTags,
			Size:       img.Size,
			Created:    img.Created,
			Containers: img.Containers,
		})
	}
	return items, nil
}

func (p *Pruner) listVolumes(ctx context.Context) ([]VolumeSummary, error) {
	result, err := p.client.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes: %w", err)
	}

	items := make([]VolumeSummary, 0, len(result.Items))
	for _, v := range result.Items {
		s := VolumeSummary{
			Name:   v.Name,
			Driver: v.Driver,
			Scope:  v.Scope,
			Size:   -1,
		}
		if v.UsageData != nil {
			s.Size = v.UsageData.Size
			s.RefCount = v.UsageData.RefCount
		}
		items = append(items, s)
	}
	return items, nil
}

func (p *Pruner) listNetworks(ctx context.Context) ([]NetworkSummary, error) {
	result, err := p.client.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	items := make([]NetworkSummary, 0, len(result.Items))
	for _, n := range result.Items {
		items = append(items, NetworkSummary{
			Name:       n.Name,
			ID:         n.ID,
			Driver:     n.Driver,
			Scope:      n.Scope,
			Internal:   n.Internal,
			Attachable: n.Attachable,
		})
	}
	return items, nil
}
