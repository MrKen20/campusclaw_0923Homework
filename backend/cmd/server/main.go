// CampusClaw api 入口：加载配置（缺失即退出）→ 等待数据库就绪 → 建表种子 → 注册路由。
// 信任边界在此：认证、授权、班级隔离、上传校验全部发生在本进程（design.md D1）。
package main

import (
	"log"
	"net/http"

	"campusclaw/backend/internal/auth"
	"campusclaw/backend/internal/config"
	"campusclaw/backend/internal/db"
	"campusclaw/backend/internal/materials"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置错误：%v", err) // 失败即停，禁止内置默认值（spec R7）
	}
	d, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("数据库配置错误：%v", err)
	}
	if err := db.WaitReady(d); err != nil {
		log.Fatalf("%v", err)
	}
	if err := db.Migrate(d); err != nil {
		log.Fatalf("建表失败：%v", err)
	}
	if err := db.Seed(d, cfg); err != nil {
		log.Fatalf("种子失败：%v", err)
	}

	authSvc := auth.NewService(d, cfg)
	matSvc := materials.NewService(d, cfg)

	// dbGuard：数据库不可用时业务接口返回 503，不得误判为会话失效 401（design.md D9）。
	dbGuard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !db.Ready(d) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"服务暂时不可用"}`))
				return
			}
			next(w, r)
		}
	}

	mux := http.NewServeMux()
	// /health 只做进程存活判定，不查数据库（design.md D9）。
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("POST /api/login", dbGuard(authSvc.Login))
	mux.HandleFunc("POST /api/logout", dbGuard(authSvc.Logout))
	mux.HandleFunc("GET /api/me", dbGuard(authSvc.Middleware(authSvc.Me)))
	mux.HandleFunc("GET /api/materials", dbGuard(authSvc.Middleware(matSvc.List)))
	mux.HandleFunc("GET /api/materials/{id}", dbGuard(authSvc.Middleware(matSvc.Detail)))
	mux.HandleFunc("GET /api/materials/{id}/file", dbGuard(authSvc.Middleware(matSvc.File)))
	mux.HandleFunc("POST /api/materials", dbGuard(authSvc.Middleware(auth.RequireTeacher(matSvc.Upload))))

	log.Printf("CampusClaw api 监听 :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}
