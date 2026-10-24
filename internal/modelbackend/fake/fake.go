// Package fake is an offline fixture, never registered by the production CLI.
package fake

import (
	"context"
	"github.com/Garden12138/xcli/internal/model"
)

type Backend struct {
	CompleteFunc func(context.Context, model.Object) (model.Object, error)
	StreamFunc   func(context.Context, model.Object, func(model.Object) error) error
	GenerateFunc func(context.Context, model.Object) (model.Object, error)
	EditFunc     func(context.Context, model.ImageEditRequest) (model.Object, error)
	EmbedFunc    func(context.Context, model.Object) (model.Object, error)
}

func (b *Backend) Complete(c context.Context, r model.Object) (model.Object, error) {
	if b.CompleteFunc == nil {
		return nil, model.Unsupported(model.Text)
	}
	return b.CompleteFunc(c, r)
}
func (b *Backend) Stream(c context.Context, r model.Object, send func(model.Object) error) error {
	if b.StreamFunc == nil {
		return model.Unsupported(model.Stream)
	}
	return b.StreamFunc(c, r, send)
}
func (b *Backend) Generate(c context.Context, r model.Object) (model.Object, error) {
	if b.GenerateFunc == nil {
		return nil, model.Unsupported(model.ImageGenerate)
	}
	return b.GenerateFunc(c, r)
}
func (b *Backend) Edit(c context.Context, r model.ImageEditRequest) (model.Object, error) {
	if b.EditFunc == nil {
		return nil, model.Unsupported(model.ImageEdit)
	}
	return b.EditFunc(c, r)
}
func (b *Backend) Embed(c context.Context, r model.Object) (model.Object, error) {
	if b.EmbedFunc == nil {
		return nil, model.Unsupported(model.Embeddings)
	}
	return b.EmbedFunc(c, r)
}
