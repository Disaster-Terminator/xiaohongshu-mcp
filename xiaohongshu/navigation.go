package xiaohongshu

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"time"
)

// 注册新文档事件后导航；图片和视频不必全部加载才开始读取数据。
func navigateDocument(page *rod.Page, url string) {
	frame, err := proto.PageGetFrameTree{}.Call(page)
	if err != nil {
		panic(err)
	}
	oldLoader := frame.FrameTree.Frame.LoaderID
	page.MustNavigate(url)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, err := proto.PageGetFrameTree{}.Call(page)
		if err != nil {
			panic(err)
		}
		if current.FrameTree.Frame.LoaderID != oldLoader {
			return
		}
		select {
		case <-page.GetContext().Done():
			panic(page.GetContext().Err())
		case <-ticker.C:
		}
	}
}
