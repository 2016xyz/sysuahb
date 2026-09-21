package routers

import (
	"net/http"

	"example.com/svcmgr/web/controllers"
	"github.com/beego/beego"
)

func Init() {
	// Handle 404
	beego.ErrorHandler("404", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
	})
	controllers.InitLogin()
	webBaseUrl := beego.AppConfig.String("web_base_url")
	if len(webBaseUrl) > 0 {
		ns := beego.NewNamespace(webBaseUrl,
			beego.NSRouter("/", &controllers.IndexController{}, "*:Index"),
			beego.NSAutoRouter(&controllers.IndexController{}),
			beego.NSAutoRouter(&controllers.LoginController{}),
			beego.NSAutoRouter(&controllers.ClientController{}),
			beego.NSAutoRouter(&controllers.AuthController{}),
			beego.NSAutoRouter(&controllers.GlobalController{}),
			beego.NSAutoRouter(&controllers.ProxyNodeController{}),
		)
		beego.AddNamespace(ns)
	} else {
		beego.Router("/", &controllers.IndexController{}, "*:Index")
		beego.AutoRouter(&controllers.IndexController{})
		beego.AutoRouter(&controllers.LoginController{})
		beego.AutoRouter(&controllers.ClientController{})
		beego.AutoRouter(&controllers.AuthController{})
		beego.AutoRouter(&controllers.GlobalController{})
		beego.AutoRouter(&controllers.ProxyNodeController{})
	}
}
