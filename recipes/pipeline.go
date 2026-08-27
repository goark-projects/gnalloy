package recipes

import (
	"fmt"
	"strings"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
)

// HandlerFactory 为每个 Channel 创建独立 Handler。
type HandlerFactory func() (channel.Handler, error)

// HandlerSpec 描述一个 Pipeline handler 的名称和构造方式。
type HandlerSpec struct {
	Name    string
	Factory HandlerFactory
}

// Use 把已有 handler 实例加入配方。仅适合无状态或调用方明确希望共享状态的 handler。
func Use(name string, handler channel.Handler) HandlerSpec {
	return HandlerSpec{Name: name, Factory: func() (channel.Handler, error) { return handler, nil }}
}

// UseFactory 把 per-channel handler 工厂加入配方。
func UseFactory(name string, factory HandlerFactory) HandlerSpec {
	return HandlerSpec{Name: name, Factory: factory}
}

// Initializer 把 handler 配方转换为 bootstrap initializer。
func Initializer(specs ...HandlerSpec) bootstrap.ChildInitializer {
	copied := append([]HandlerSpec(nil), specs...)
	return func(ch channel.Channel) error {
		return Apply(ch, copied...)
	}
}

// Apply 按顺序装配 handlers；失败时回滚本次已安装 handler。
func Apply(ch channel.Channel, specs ...HandlerSpec) error {
	if ch == nil {
		return fmt.Errorf("%w: nil channel", ErrInvalidRecipe)
	}
	pipeline := ch.Pipeline()
	if pipeline == nil {
		return fmt.Errorf("%w: nil pipeline", ErrInvalidRecipe)
	}
	if err := validateSpecs(pipeline, specs); err != nil {
		return err
	}
	added := make([]string, 0, len(specs))
	for _, spec := range specs {
		name := strings.TrimSpace(spec.Name)
		handler, err := spec.Factory()
		if err != nil {
			removeAdded(pipeline, added)
			return err
		}
		if handler == nil {
			removeAdded(pipeline, added)
			return fmt.Errorf("%w: nil handler %s", ErrInvalidRecipe, name)
		}
		if err := pipeline.AddLast(name, handler); err != nil {
			removeAdded(pipeline, added)
			return err
		}
		added = append(added, name)
	}
	return nil
}

func validateSpecs(pipeline *channel.Pipeline, specs []HandlerSpec) error {
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		name := strings.TrimSpace(spec.Name)
		if name == "" || spec.Factory == nil {
			return ErrInvalidRecipe
		}
		if _, ok := seen[name]; ok {
			return channel.ErrDuplicateHandler
		}
		if _, ok := pipeline.Context(name); ok {
			return channel.ErrDuplicateHandler
		}
		seen[name] = struct{}{}
	}
	return nil
}

func removeAdded(pipeline *channel.Pipeline, names []string) {
	for i := len(names) - 1; i >= 0; i-- {
		_ = pipeline.Remove(names[i])
	}
}

func appendSpecs(base []HandlerSpec, extra []HandlerSpec) []HandlerSpec {
	out := make([]HandlerSpec, 0, len(base)+len(extra))
	out = append(out, base...)
	out = append(out, extra...)
	return out
}
