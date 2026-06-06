package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// جلب التوكن السري من إعدادات الـ Railway أو استخدام الافتراضي للدخول للوحة
func getAdminToken() string {
	token := os.Getenv("ADMIN_TOKEN")
	if token == "" {
		return "Ahmed12@12" // التوكن الافتراضي
	}
	return token
}

// ملف حفظ المشتركين في مجلد Railway المؤقت لضمان عدم مسح البيانات عند إعادة التشغيل
const DBFile = "/tmp/database.json"

type ServerCreds struct {
	URL  string `json:"url"`
	User string `json:"user"`
	Pass string `json:"pass"`
}

type ServerConfig struct {
	Server1 ServerCreds `json:"server1"`
	Server2 ServerCreds `json:"server2"`
	Server3 ServerCreds `json:"server3"`
}

type Subscriber struct {
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Phone      string    `json:"phone"`
	ExpireTime time.Time `json:"expire_time"`
	IsActive   bool      `json:"is_active"`
}

type AppData struct {
	Config    ServerConfig          `json:"config"`
	Customers map[string]Subscriber `json:"customers"`
}

var (
	dbMutex sync.RWMutex
	appData = AppData{
		Config: ServerConfig{
			Server1: ServerCreds{URL: "http://m12m5678.xyz:2082", User: "Jamalnajjar2026", Pass: "462546564152"},
			Server2: ServerCreds{URL: "", User: "", Pass: ""},
			Server3: ServerCreds{URL: "", User: "", Pass: ""},
		},
		Customers: map[string]Subscriber{},
	}
)

func saveDB() {
	data, err := json.MarshalIndent(appData, "", "  ")
	if err == nil {
		tmpFile := DBFile + ".tmp"
		if err := os.WriteFile(tmpFile, data, 0644); err == nil {
			os.Rename(tmpFile, DBFile)
		}
	}
}

func loadDB() {
	data, err := os.ReadFile(DBFile)
	if err == nil {
		json.Unmarshal(data, &appData)
	}
}

func init() {
	loadDB()
}

// لوحة التحكم الخاصة بك لإدارة المشتركين وإضافة البيانات
func handleDashboard(w http.ResponseWriter, r *http.Request) {
	adminToken := getAdminToken()
	token := r.URL.Query().Get("token")
	
	if token != adminToken {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "<h1 style='color:red; text-align:center; margin-top:50px;'>عذراً، الرمز السري خاطئ!</h1>")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	dbMutex.RLock()
	totalCount := len(appData.Customers)
	activeCount := 0
	expiredCount := 0

	for _, sub := range appData.Customers {
		if !sub.IsActive || time.Now().After(sub.ExpireTime) {
			expiredCount++
		} else {
			activeCount++
		}
	}
	cfg := appData.Config
	dbMutex.RUnlock()

	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	currentHost := fmt.Sprintf("%s://%s", scheme, r.Host)

	fmt.Fprintf(w, `
	<!DOCTYPE html>
	<html lang="ar" dir="rtl">
	<head>
		<meta charset="UTF-8">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<title>لوحة إدارة IPTV الحرة</title>
		<script src="https://cdn.tailwindcss.com"></script>
	</head>
	<body class="bg-[#0b0c10] text-gray-100 p-6">
		<div class="max-w-5xl mx-auto">
			<header class="mb-6 bg-[#1f2833] p-4 rounded-2xl border border-gray-800 text-center">
				<h1 class="text-2xl font-black text-blue-400">BOSS IPTV (نسخة بدون حظر أجهزة)</h1>
				<p class="text-xs text-gray-400 mt-1">يمكنك تشغيل الحسابات على أكثر من جهاز في نفس الوقت بحرية تامة</p>
				<div class="grid grid-cols-2 gap-4 mt-4 max-w-sm mx-auto">
					<div class="bg-black/40 p-2 rounded-xl">إجمالي الأكواد: <span class="text-blue-400 font-bold">%d</span></div>
					<div class="bg-black/40 p-2 rounded-xl">النشطين: <span class="text-green-400 font-bold">%d</span></div>
				</div>
			</header>

			<div class="bg-[#1f2833] p-5 rounded-2xl border border-gray-800 mb-6">
				<h3 class="text-sm font-bold text-blue-400 mb-3">✨ إنشاء مشترك جديد</h3>
				<form action="/api/add" method="GET" class="grid grid-cols-1 sm:grid-cols-4 gap-3">
					<input type="hidden" name="token" value="%s">
					<input type="text" name="code" placeholder="كود الحساب (يوزر وباص بنفس الوقت)" class="bg-black/50 p-3 rounded-xl border border-gray-700 text-center text-sm" required>
					<input type="text" name="name" placeholder="اسم الزبون" class="bg-black/50 p-3 rounded-xl border border-gray-700 text-center text-sm" required>
					<input type="number" name="days" placeholder="عدد الأيام (مثلاً 30 أو 365)" class="bg-black/50 p-3 rounded-xl border border-gray-700 text-center text-sm" required>
					<button type="submit" class="bg-blue-600 hover:bg-blue-700 text-white font-bold rounded-xl text-sm transition">حفظ الحساب</button>
				</form>
			</div>

			<div class="bg-[#1f2833] rounded-2xl border border-gray-800 overflow-hidden">
				<table class="w-full text-center text-sm">
					<thead class="bg-[#141c24] text-gray-400">
						<tr>
							<th class="p-4">الكود / الحساب</th>
							<th class="p-4">الاسم</th>
							<th class="p-4">تاريخ الانتهاء</th>
							<th class="p-4">الحالة</th>
							<th class="p-4">الإجراءات</th>
						</tr>
					</thead>
					<tbody>`, totalCount, activeCount, adminToken)

	dbMutex.RLock()
	for _, sub := range appData.Customers {
		statusStr := "<span class='text-green-400 bg-green-500/10 px-2 py-1 rounded border border-green-500/20'>نشط</span>"
		if !sub.IsActive || time.Now().After(sub.ExpireTime) {
			statusStr = "<span class='text-red-400 bg-red-500/10 px-2 py-1 rounded border border-red-500/20'>منتهي/معطل</span>"
		}
		formattedExpiry := sub.ExpireTime.Format("2006-01-02")

		fmt.Fprintf(w, `
						<tr class="border-b border-gray-800 hover:bg-[#141c24]/50">
							<td class="p-4 font-mono font-bold text-blue-400">%s</td>
							<td class="p-4">%s</td>
							<td class="p-4 font-mono text-xs">%s</td>
							<td class="p-4">%s</td>
							<td class="p-4 flex justify-center gap-2">
								<a href="/api/delete?token=%s&code=%s" class="bg-red-600/20 text-red-400 border border-red-500/30 px-3 py-1 rounded-lg hover:bg-red-600 hover:text-white transition text-xs">حذف</a>
							</td>
						</tr>`, sub.Code, sub.Name, formattedExpiry, statusStr, adminToken, sub.Code)
	}
	dbMutex.RUnlock()

	fmt.Fprintf(w, `
					</tbody>
				</table>
			</div>

			<div class="bg-[#1f2833] p-5 rounded-2xl border border-gray-800 mt-6 text-center">
				<h3 class="text-sm font-bold text-amber-400 mb-3">⚙️ بيانات السيرفر المصدر الأساسي (المحلي أو الخارجي)</h3>
				<p class="text-xs text-gray-400 mb-4">هذه هي البيانات التي يتم سحب القنوات منها وتمريرها للمشتركين</p>
				<div class="grid grid-cols-1 sm:grid-cols-3 gap-3 bg-black/40 p-4 rounded-xl text-xs font-mono text-gray-300">
					<div>رابط السيرفر: <span class="text-blue-400">%s</span></div>
					<div>اليوزر الرئيسي: <span class="text-blue-400">%s</span></div>
					<div>الباص الرئيسي: <span class="text-blue-400">%s</span></div>
				</div>
			</div>
		</div>
	</body>
	</html>`, cfg.Server1.URL, cfg.Server1.User, cfg.Server1.Pass)
}

func handleAPI(w http.ResponseWriter, r *http.Request) {
	adminToken := getAdminToken()
	token := r.URL.Query().Get("token")
	if token != adminToken {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	path := r.URL.Path
	dbMutex.Lock()

	if path == "/api/add" {
		code := r.URL.Query().Get("code")
		name := r.URL.Query().Get("name")
		var days int
		fmt.Sscanf(r.URL.Query().Get("days"), "%d", &days)

		if code != "" && name != "" && days > 0 {
			appData.Customers[code] = Subscriber{
				Code:       code,
				Name:       name,
				ExpireTime: time.Now().Add(time.Duration(days) * 24 * time.Hour),
				IsActive:   true,
			}
			saveDB()
		}
	} else if path == "/api/delete" {
		code := r.URL.Query().Get("code")
		delete(appData.Customers, code)
		saveDB()
	}

	dbMutex.Unlock()
	http.Redirect(w, r, "/dashboard?token="+adminToken, http.StatusSeeOther)
}

// دالة تحويل وضخ البث والتخطي الكامل لأي نوع حظر أو فحص أجهزة
func handleIPTVRedirect(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	password := r.URL.Query().Get("password")
	action := r.URL.Query().Get("action")

	dbMutex.RLock()
	sub, exists := appData.Customers[username]
	cfg := appData.Config
	dbMutex.RUnlock()

	// التأكد فقط من وجود الحساب وصلاحيته، وبدون حفظ أي آي بي أو حظر
	if !exists || password != username || !sub.IsActive || time.Now().After(sub.ExpireTime) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"user_info":{"auth":0,"status":"Expired or Disabled"}}`))
		return
	}

	// تمرير الطلب فوراً ومباشرة للسيرفر الأساسي
	var target = cfg.Server1
	var redirectURL string

	if action != "" {
		redirectURL = fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=%s", target.URL, target.User, target.Pass, action)
	} else {
		redirectURL = fmt.Sprintf("%s/get.php?username=%s&password=%s&output=ts", target.URL, target.User, target.Pass)
	}

	// إلغاء كاش التخزين لتحديث البث فوراً
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	
	// استخدام نظام الـ HTTP Redirect لتخفيف العبء عن ريلواي وتوجيه المشترك للبث مباشرة
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

func main() {
	http.HandleFunc("/player_api.php", handleIPTVRedirect)
	http.HandleFunc("/get.php", handleIPTVRedirect)
	http.HandleFunc("/dashboard", handleDashboard)
	http.HandleFunc("/api/add", handleAPI)
	http.HandleFunc("/api/delete", handleAPI)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
