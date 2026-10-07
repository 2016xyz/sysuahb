package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beego/beego/context"
	"github.com/beego/beego/session"
)

// newMemSessionStore 用 beego 内存 session 后端造一个真实 Store,
// 供测试直接注入 controller 的 CruSession。
func newMemSessionStore(t *testing.T, values map[interface{}]interface{}) session.Store {
	t.Helper()
	cfg := &session.ManagerConfig{
		CookieName:      "beegosessionID",
		EnableSetCookie: false,
		Gclifetime:      3600,
		ProviderConfig:  "{}",
	}
	mgr, err := session.NewManager("memory", cfg)
	if err != nil {
		t.Fatalf("new memory session manager: %v", err)
	}
	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	store, err := mgr.SessionStart(w, r)
	if err != nil {
		t.Fatalf("session start: %v", err)
	}
	for k, v := range values {
		_ = store.Set(k, v)
	}
	return store
}

// TestProxyNodePrepareDeniesNonAdmin 锁定越权修复:
// 代理节点是全局出口配置, 任何非管理员(客户端用户)都不允许触碰。
// 修复前 ProxyNodeController 未覆盖 Prepare, BaseController.CheckUserAuth 只保护
// client/index 控制器, 于是 allow_user_login 的客户端用户能读写/删除/注入代理节点
// —— 可注入指向攻击者的节点劫持其他用户流量(实测已复现)。
func TestProxyNodePrepareDeniesNonAdmin(t *testing.T) {
	ctrl := &ProxyNodeController{}
	ctx := context.NewContext()
	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1/proxynode/list", nil)
	ctx.Reset(w, r)
	ctrl.Init(ctx, "ProxyNodeController", "List", ctrl)
	ctrl.Ctx.Input.CruSession = newMemSessionStore(t, map[interface{}]interface{}{
		"auth":     true,
		"isAdmin":  false,
		"clientId": 7,
		"username": "secuser",
	})

	aborted := false
	func() {
		defer func() {
			if recover() != nil {
				// StopRun panics with ErrAbort —— Prepare 拒绝的预期行为。
				aborted = true
			}
		}()
		ctrl.Prepare()
	}()
	if !aborted {
		t.Fatal("Prepare did not abort a non-admin request; authorization regression")
	}
}

func TestProxyNodePrepareAllowsAdmin(t *testing.T) {
	ctrl := &ProxyNodeController{}
	ctx := context.NewContext()
	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1/proxynode/list", nil)
	ctx.Reset(w, r)
	ctrl.Init(ctx, "ProxyNodeController", "List", ctrl)
	ctrl.Ctx.Input.CruSession = newMemSessionStore(t, map[interface{}]interface{}{
		"auth":    true,
		"isAdmin": true,
	})

	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Fatalf("Prepare aborted an admin request: %v", p)
			}
		}()
		ctrl.Prepare()
	}()
}
