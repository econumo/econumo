package imports

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateSource is idempotent per (user, provider): the shortcut setup can
// be re-run without leaving duplicate sources behind.
func (s *Service) CreateSource(ctx context.Context, userID vo.Id, req model.CreateImportSourceRequest) (*model.CreateImportSourceResult, error) {
	var out *model.CreateImportSourceResult
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		src, err := s.repo.GetSourceByUserProvider(ctx, userID, req.Provider)
		if err != nil {
			if _, ok := errs.AsNotFound(err); !ok {
				return err
			}
			now := s.clk.Now().UTC()
			src = &model.ImportSource{
				ID: vo.NewId(), UserID: userID, Provider: req.Provider, Name: req.Name,
				Status: model.ImportSourceStatusActive, CreatedAt: now, UpdatedAt: now,
			}
			if err := s.repo.InsertSource(ctx, src); err != nil {
				return err
			}
		}
		item, err := s.sourceResult(ctx, src)
		if err != nil {
			return err
		}
		out = &model.CreateImportSourceResult{Item: *item}
		return nil
	})
	return out, err
}

func (s *Service) GetSourceList(ctx context.Context, userID vo.Id) (*model.GetImportSourceListResult, error) {
	sources, err := s.repo.ListSourcesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &model.GetImportSourceListResult{Items: make([]model.ImportSourceResult, 0, len(sources))}
	for i := range sources {
		item, err := s.sourceResult(ctx, &sources[i])
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, *item)
	}
	return out, nil
}

// DeleteSource removes the source and, by FK cascade, its events, runs,
// links and card mappings; imported transactions stay.
func (s *Service) DeleteSource(ctx context.Context, userID vo.Id, req model.DeleteImportSourceRequest) (*model.GetImportSourceListResult, error) {
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		src, err := s.ownedSource(ctx, userID, req.Id)
		if err != nil {
			return err
		}
		return s.repo.DeleteSource(ctx, src.ID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetSourceList(ctx, userID)
}
