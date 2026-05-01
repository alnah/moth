package browser

import (
	"context"
	"fmt"

	"github.com/go-rod/rod/lib/proto"
)

func (worker *rodWorker) AccessibilityTree(
	ctx context.Context,
	request AccessibilityRequest,
) (AccessibilityTree, error) {
	page, err := worker.selectedPage(ctx, request.ProfileName, request.SessionName, request.PageID)
	if err != nil {
		return AccessibilityTree{}, err
	}
	var depth *int
	if request.MaxDepth > 0 {
		depth = &request.MaxDepth
	}
	result, err := proto.AccessibilityGetFullAXTree{Depth: depth}.Call(page.Context(ctx))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return AccessibilityTree{}, ctxErr
		}
		return AccessibilityTree{}, fmt.Errorf("get accessibility tree: %w", err)
	}
	nodes := make([]AccessibilityNode, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		if node.Ignored {
			continue
		}
		nodes = append(nodes, AccessibilityNode{Role: axValueString(node.Role), Name: axValueString(node.Name)})
	}
	return AccessibilityTree{Nodes: nodes}, nil
}

func axValueString(value *proto.AccessibilityAXValue) string {
	if value == nil {
		return ""
	}
	return value.Value.Str()
}
