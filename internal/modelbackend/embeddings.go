package modelbackend

import (
	"context"
	"github.com/Garden12138/xcli/internal/model"
)

func (b *OpenAI) Embed(ctx context.Context, req model.Object) (model.Object, error) {
	return b.jsonRequest(ctx, "/embeddings", req)
}
