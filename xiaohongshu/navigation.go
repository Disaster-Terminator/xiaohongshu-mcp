package xiaohongshu

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"time"
)

// 注册新文档事件后导航；图片和视频不必全部加载才开始读取数据。
func navigateDocument(page *rod.Page, url string) {
	if err := navigateDocumentChecked(page, url); err != nil {
		panic(err)
	}
}

func navigateDocumentChecked(page *rod.Page, url string) error {
	frame, err := proto.PageGetFrameTree{}.Call(page)
	if err != nil {
		return err
	}
	oldLoader := frame.FrameTree.Frame.LoaderID
	if err := page.Navigate(url); err != nil {
		return err
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, err := proto.PageGetFrameTree{}.Call(page)
		if err != nil {
			return err
		}
		if current.FrameTree.Frame.LoaderID != oldLoader {
			return nil
		}
		select {
		case <-page.GetContext().Done():
			return page.GetContext().Err()
		case <-ticker.C:
		}
	}
}
