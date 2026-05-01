package browser

import (
	"context"
	"fmt"

	"github.com/go-rod/rod/lib/proto"
)

func (worker *rodWorker) Click(ctx context.Context, request InteractionRequest) error {
	page, err := worker.selectedPage(ctx, request.ProfileName, request.SessionName, request.PageID)
	if err != nil {
		return err
	}
	element, err := page.Element(request.Selector)
	if err != nil {
		return fmt.Errorf("find click target: %w", err)
	}
	if err := element.Context(ctx).Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("click target: %w", err)
	}
	return nil
}

func (worker *rodWorker) Input(ctx context.Context, request InputRequest) error {
	page, err := worker.selectedPage(ctx, request.ProfileName, request.SessionName, request.PageID)
	if err != nil {
		return err
	}
	element, err := page.Element(request.Selector)
	if err != nil {
		return fmt.Errorf("find input target: %w", err)
	}
	if err := element.Context(ctx).Input(request.Text); err != nil {
		return fmt.Errorf("input target: %w", err)
	}
	return nil
}

func (worker *rodWorker) Wait(ctx context.Context, request WaitRequest) error {
	page, err := worker.selectedPage(ctx, request.ProfileName, request.SessionName, request.PageID)
	if err != nil {
		return err
	}
	if request.State == WaitVisible {
		element, err := page.Element(request.Selector)
		if err != nil {
			return fmt.Errorf("find wait target: %w", err)
		}
		if err := element.Context(ctx).WaitVisible(); err != nil {
			return fmt.Errorf("wait target visible: %w", err)
		}
		return nil
	}
	if err := page.WaitElementsMoreThan(request.Selector, 0); err != nil {
		return fmt.Errorf("wait target attached: %w", err)
	}
	return nil
}
