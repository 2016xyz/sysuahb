package controllers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 手机端抽屉式侧边栏靠切换 <body> 上的 `mini-navbar` 类来打开:
//   - inspinia.js:  $("body").toggleClass("mini-navbar")
//   - layout.html:  遮罩 onclick="document.body.classList.remove('mini-navbar')"
//
// 样式表曾把打开状态写成后代选择器 `body:not(.login-page) .mini-navbar ...`,
// 语义变成"body 的后代元素带 mini-navbar"。而该类实际加在 body 自身上,
// 于是规则永不匹配, 侧边栏一直停在 translateX(-110%) 屏外 —— 手机上点汉堡
// 毫无反应。正确写法必须把 .mini-navbar 与 body 复合:
//
//	body:not(.login-page).mini-navbar ...
//
// 本测试锁死这一点, 防止日后被"顺手加空格"改回去。
func TestMobileSidebarOpenStateSelectorIsCompound(t *testing.T) {
	css := readProjectFile(t, "web/static/css/style.css")

	// 命中即回归: 打开状态被写成了后代选择器。
	buggy := "body:not(.login-page) .mini-navbar"
	if n := strings.Count(css, buggy); n > 0 {
		t.Fatalf("%q appears %d time(s): .mini-navbar is a descendant selector here, "+
			"but the class is set on <body> itself, so these rules can never match and "+
			"the mobile sidebar stays off-screen. Use body:not(.login-page).mini-navbar", buggy, n)
	}

	// 必须存在复合写法, 否则抽屉同样打不开。
	correct := "body:not(.login-page).mini-navbar"
	if n := strings.Count(css, correct); n == 0 {
		t.Fatalf("%q not found: nothing opens the mobile sidebar", correct)
	}
}

// 打开状态的规则必须真的把侧边栏移回屏幕内, 只加 visibility 是不够的。
func TestMobileSidebarOpenStateResetsTransform(t *testing.T) {
	css := readProjectFile(t, "web/static/css/style.css")

	idx := strings.Index(css, "body:not(.login-page).mini-navbar nav.navbar-static-side")
	if idx < 0 {
		t.Fatal("open-state rule for nav.navbar-static-side not found")
	}
	// 取该规则的声明块。
	rest := css[idx:]
	end := strings.Index(rest, "}")
	if end < 0 {
		t.Fatal("malformed css: rule block is not closed")
	}
	block := rest[:end]

	if !strings.Contains(block, "translateX(0)") {
		t.Fatalf("open-state rule must reset the transform to translateX(0), got: %q", block)
	}
	if !strings.Contains(block, "visible") {
		t.Fatalf("open-state rule must make the sidebar visible, got: %q", block)
	}
}

// 关闭状态必须仍然把侧边栏推出屏外, 否则会常驻遮挡页面。
func TestMobileSidebarClosedStatePushesOffscreen(t *testing.T) {
	css := readProjectFile(t, "web/static/css/style.css")

	idx := strings.Index(css, "body:not(.login-page) nav.navbar-static-side")
	if idx < 0 {
		t.Fatal("base (closed) rule for nav.navbar-static-side not found")
	}
	rest := css[idx:]
	end := strings.Index(rest, "}")
	if end < 0 {
		t.Fatal("malformed css: rule block is not closed")
	}
	block := rest[:end]

	if !strings.Contains(block, "hidden") {
		t.Fatalf("closed-state rule must set visibility:hidden, got: %q", block)
	}
	if !strings.Contains(block, "translateX(-110%)") {
		t.Fatalf("closed-state rule must push the sidebar off-screen, got: %q", block)
	}
}

// 遮罩的打开状态用同一个类, 必须一并复合, 否则遮罩不出现。
func TestMobileNavBackdropUsesCompoundSelector(t *testing.T) {
	css := readProjectFile(t, "web/static/css/style.css")
	if !strings.Contains(css, "body:not(.login-page).mini-navbar .mobile-nav-backdrop") {
		t.Fatal("backdrop open-state rule must be compound too, " +
			"otherwise tapping the hamburger shows the drawer without its backdrop")
	}
}

// readProjectFile 读取仓库内文件; 测试在 web/controllers 下运行, 故向上回溯。
func readProjectFile(t *testing.T, rel string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, rel)
		if b, err := os.ReadFile(candidate); err == nil {
			return string(b)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate %s from %s", rel, dir)
	return ""
}
