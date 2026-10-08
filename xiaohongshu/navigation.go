package xiaohongshu

import (
	"context"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// 注册新文档事件后导航；图片和视频不必全部加载才开始读取数据。
func navigateDocument(page *rod.Page, url string) {
	ctx, cancel := context.WithCancel(page.GetContext())
	defer cancel()
	pp := page.Context(ctx)
	frame, err := proto.PageGetFrameTree{}.Call(pp)
	if err != nil {
		panic(err)
	}
	oldLoader := frame.FrameTree.Frame.LoaderID
	if err := (proto.PageSetLifecycleEventsEnabled{Enabled: true}).Call(pp); err != nil {
		panic(err)
	}
	defer func() { _ = (proto.PageSetLifecycleEventsEnabled{Enabled: false}).Call(page) }()
	wait := pp.EachEvent(func(e *proto.PageLifecycleEvent) bool {
		return e.FrameID == page.FrameID && e.LoaderID != oldLoader &&
			e.Name == proto.PageLifecycleEventNameDOMContentLoaded
	})
	pp.MustNavigate(url)
	wait()
}
