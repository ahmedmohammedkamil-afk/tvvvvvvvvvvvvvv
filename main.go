package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// جلب التوكن السري من إعدادات الـ Railway أو استخدام الافتراضي
func getAdminToken() string {
	token := os.Getenv("ADMIN_TOKEN")
	if token == "" {
		return "Ahmed12@12" // التوكن الافتراضي في حال لم تقم بتعيينه في الموقع
	}
	return token
}

// لتجنب مسح البيانات عند توقف السيرفر المجاني في Railway
// نستخدم مجلد /tmp المؤقت المتاح للكتابة دائماً
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
	LastIP     string    `json:"last_ip"`
	LastUA     string    `json:"last_ua"`
	LastSeen   time.Time `json:"last_seen"`
}

type AppData struct {
	Config    ServerConfig          `json:"config"`
	Customers map[string]Subscriber `json:"customers"`
}

type SecurityAndCache struct {
	LoginAttempts map[string]int
	BlockedIPs    map[string]time.Time
	ServerStatus  map[string]bool
	Mutex         sync.Mutex
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
	scData = SecurityAndCache{
		LoginAttempts: make(map[string]int),
		BlockedIPs:    make(map[string]time.Time),
		ServerStatus:  make(map[string]bool),
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

func getIP(r *http.Request) string {
	// منصة Railway تضع آي بي العميل الحقيقي في هذا الهيدر دائماً
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	if strings.Contains(ip, ",") {
		ip = strings.Split(ip, ",")[0]
	}
	if strings.Contains(ip, ":") {
		ip = strings.Split(ip, ":")[0]
	}
	return strings.TrimSpace(ip)
}

func isServerAlive(url string) bool {
	if url == "" {
		return false
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusFound
}

func init() {
	loadDB()

	go func() {
		for {
			dbMutex.RLock()
			s1, s2, s3 := appData.Config.Server1.URL, appData.Config.Server2.URL, appData.Config.Server3.URL
			dbMutex.RUnlock()

			scData.Mutex.Lock()
			scData.ServerStatus[s1] = isServerAlive(s1)
			scData.ServerStatus[s2] = isServerAlive(s2)
			scData.ServerStatus[s3] = isServerAlive(s3)
			scData.Mutex.Unlock()

			dbMutex.Lock()
			changed := false
			for code, sub := range appData.Customers {
				if time.Now().After(sub.ExpireTime.Add(48 * time.Hour)) {
					delete(appData.Customers, code)
					changed = true
				}
			}
			if changed {
				saveDB()
			}
			dbMutex.Unlock()

			time.Sleep(30 * time.Second)
		}
	}()
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	ip := getIP(r)
	adminToken := getAdminToken()

	scData.Mutex.Lock()
	if unblockTime, blocked := scData.BlockedIPs[ip]; blocked {
		if time.Now().Before(unblockTime) {
			scData.Mutex.Unlock()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, "<h1 style='color:red; text-align:center; margin-top:50px;'>تم حظرك مؤقتاً لـ 60 دقيقة!</h1>")
			return
		}
		delete(scData.BlockedIPs, ip)
		scData.LoginAttempts[ip] = 0
	}

	token := r.URL.Query().Get("token")
	if token != adminToken {
		scData.LoginAttempts[ip]++
		if scData.LoginAttempts[ip] >= 5 {
			scData.BlockedIPs[ip] = time.Now().Add(1 * time.Hour)
			scData.Mutex.Unlock()
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, "تم حظرك بسبب كثرة التخمين الخاطئ.")
			return
		}
		scData.Mutex.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "<h1 style='color:red; text-align:center; margin-top:50px;'>عذراً، الرمز السري خاطئ! متبقي لك %d محاولات.</h1>", 5-scData.LoginAttempts[ip])
		return
	}
	scData.LoginAttempts[ip] = 0
	scData.Mutex.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	dbMutex.RLock()
	totalCount := len(appData.Customers)
	activeCount := 0
	expiringSoonCount := 0
	expiredCount := 0

	for _, sub := range appData.Customers {
		if !sub.IsActive || time.Now().After(sub.ExpireTime) {
			expiredCount++
		} else {
			activeCount++
			if time.Until(sub.ExpireTime) < 7*24*time.Hour {
				expiringSoonCount++
			}
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
	<html lang="ar" dir="rtl" class="dark">
	<head>
		<meta charset="UTF-8">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<title>لوحة إدارة IPTV الذكية</title>
		<script src="https://cdn.tailwindcss.com"></script>
		<script src="https://cdnjs.cloudflare.com/ajax/libs/html2canvas/1.4.1/html2canvas.min.js"></script>
		<script>
			tailwind.config = { darkMode: 'class' }
		</script>
		<style>
			body { font-family: system-ui, sans-serif; transition: background-color 0.3s, color 0.3s; }
			.card-bg { background: linear-gradient(135deg, #090a0f 0%%, #121620 100%%); }
		</style>
		<script>
			function toggleMode() {
				const html = document.documentElement;
				if (html.classList.contains('dark')) {
					html.classList.remove('dark');
					localStorage.setItem('theme', 'light');
				} else {
					html.classList.add('dark');
					localStorage.setItem('theme', 'dark');
				}
			}
			window.onload = function() {
				if (localStorage.getItem('theme') === 'light') {
					document.documentElement.classList.remove('dark');
				}
			}
			function autoGen() {
				const hex = "0123456789ABCDEF";
				let result = "BOSS-";
				for (let i = 0; i < 8; i++) { result += hex[Math.floor(Math.random() * 16)]; }
				document.getElementById("code_input").value = result;
			}
			function setDays(d) {
				document.getElementById("days_input").value = d;
			}
			function getMsg(name, code, host) {
				return "📺 اشتراك IPTV مميز من (أحمد آل جبور):\n\n" +
				       "👤 اسم المشترك: " + name + "\n\n" +
				       "🌐 رابط السيرفر (Host):\n" + host + "\n\n" +
				       "👤 اسم المستخدم (User):\n" + code + "\n" +
				       "🔒 كلمة المرور (Pass):\n" + code + "\n\n" +
				       "⚠️ تنبيه: الحساب مخصص لـ (جهاز واحد فقط). تشغيله على أكثر من جهاز يغلق الحساب تلقائياً!\n\n" +
				       "✈️ الدعم الفني تليكرام: @ph_7amo\n\n" +
				       "💡 ضع هذه البيانات في تطبيق الـ IPTV الخاص بك ومشاهدة ممتعة!";
			}
			function copyActivation(name, code, host) {
				navigator.clipboard.writeText(getMsg(name, code, host));
				alert("تم نسخ رسالة التفعيل الجاهزة بنجاح!");
			}
			function sendWhatsApp(phone, name, code, host) {
				if(!phone || phone.trim() === "") {
					alert("لم يتم إدخل رقم هاتف لهذا الزبون!");
					return;
				}
				let cleanPhone = phone.replace(/[^0-9]/g, "");
				if (cleanPhone.startsWith("07")) { cleanPhone = "964" + cleanPhone.substring(1); }
				const url = "https://api.whatsapp.com/send?phone=" + cleanPhone + "&text=" + encodeURIComponent(getMsg(name, code, host));
				window.open(url, '_blank');
			}

			function generateCardImage(name, code, host, expiry) {
				document.getElementById("card_name").innerText = name;
				document.getElementById("card_host").value = host;
				document.getElementById("card_user").value = code;
				document.getElementById("card_pass").value = code;
				document.getElementById("card_expiry").innerText = "تاريخ الانتهاء: " + expiry;
				
				const container = document.getElementById("card_container");
				container.classList.remove("hidden");

				setTimeout(() => {
					html2canvas(document.getElementById("iptv_premium_card"), { scale: 3, useCORS: true }).then(canvas => {
						const link = document.createElement('a');
						link.download = 'IPTV_' + name + '.png';
						link.href = canvas.toDataURL('image/png');
						link.click();
						container.classList.add("hidden");
					});
				}, 200);
			}
		</script>
	</head>
	<body class="bg-slate-50 text-slate-900 dark:bg-[#0b0c10] dark:text-gray-100 p-3 sm:p-6 select-none">
		
		<div id="card_container" class="hidden fixed inset-0 bg-black/80 z-50 flex items-center justify-center p-4">
			<div id="iptv_premium_card" class="card-bg w-[360px] p-6 rounded-3xl border border-blue-500/40 text-white shadow-2xl relative overflow-hidden text-right" dir="rtl">
				<div class="absolute -right-10 -top-10 w-32 h-32 bg-blue-500/10 rounded-full blur-2xl"></div>
				<div class="absolute -left-10 -bottom-10 w-32 h-32 bg-purple-500/10 rounded-full blur-2xl"></div>
				
				<div class="text-center border-b border-gray-800 pb-3 mb-4">
					<div class="text-3xl mb-1">📺</div>
					<h2 class="text-lg font-black text-blue-400 tracking-wide">أحمد آل جبور</h2>
					<p class="text-[10px] text-gray-500 tracking-widest font-mono uppercase mt-0.5">Premium IPTV Provider</p>
				</div>
				
				<div class="bg-blue-500/5 border border-blue-500/10 rounded-xl p-3 mb-3 text-center">
					<span class="text-[11px] text-gray-400 block mb-0.5">اسم المشترك</span>
					<span id="card_name" class="text-base font-black text-gray-100">جاري التحميل...</span>
				</div>
				
				<div class="space-y-3">
					<div>
						<label class="text-[11px] text-blue-400 block mb-1">🌐 رابط السيرفر (Host):</label>
						<input id="card_host" type="text" class="w-full bg-black/50 border border-gray-800 text-xs p-2 rounded-xl text-left font-mono text-gray-300" readonly>
					</div>
					<div>
						<label class="text-[11px] text-blue-400 block mb-1">👤 اسم المستخدم (User):</label>
						<input id="card_user" type="text" class="w-full bg-black/50 border border-gray-800 text-sm p-2 rounded-xl text-center font-mono font-bold text-yellow-400 tracking-wider" readonly>
					</div>
					<div>
						<label class="text-[11px] text-blue-400 block mb-1">🔒 كلمة المرور (Pass):</label>
						<input id="card_pass" type="text" class="w-full bg-black/50 border border-gray-800 text-sm p-2 rounded-xl text-center font-mono font-bold text-yellow-400 tracking-wider" readonly>
					</div>
				</div>
				
				<div class="mt-4 text-center bg-red-500/10 border border-red-500/20 py-1 rounded-lg">
					<span class="text-[10px] text-red-400 font-bold">⚠️ الحساب مخصص لجهاز واحد فقط (الحظر تلقائي)</span>
				</div>
				
				<div class="mt-4 pt-3 border-t border-gray-800 space-y-1.5 text-[11px]">
					<div class="flex justify-between items-center text-gray-400 font-mono">
						<span id="card_expiry">تاريخ الانتهاء: 2026-12-12</span>
						<span class="text-green-500 font-sans font-bold">نشط ✓</span>
					</div>
					<div class="flex justify-between items-center bg-black/30 p-1.5 rounded-lg border border-gray-800/60 font-mono">
						<span class="text-gray-400 text-[10px]">✈️ الدعم الفني تليكرام:</span>
						<span class="text-blue-400 font-bold">@ph_7amo</span>
					</div>
				</div>
			</div>
		</div>

		<div class="max-w-5xl mx-auto">
			
			<div class="flex justify-end mb-4">
				<button onclick="toggleMode()" class="bg-slate-200 dark:bg-[#1f2833] text-xs font-bold px-4 py-2 rounded-xl border border-slate-300 dark:border-gray-800 shadow transition flex items-center gap-2">
					🌓 تبديل المظهر (ليلي / نهاري)
				</button>
			</div>

			<header class="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6 bg-white dark:bg-[#1f2833] p-4 rounded-2xl border border-slate-200 dark:border-gray-800 shadow-xl">
				<div class="col-span-2 sm:col-span-4 mb-2">
					<h1 class="text-xl sm:text-2xl font-black text-blue-500 tracking-wide">BOSS IPTV CONTROL</h1>
					<p class="text-xs text-slate-500 dark:text-gray-400 mt-1">النظام الذكي المؤتمت بالكامل لإدارة العملاء والواتساب المباشر والتأمين الذاتي</p>
				</div>
				<div class="bg-slate-100 dark:bg-[#0b0c10] px-4 py-2 rounded-xl border border-slate-200 dark:border-gray-800/80 text-center">
					<span class="text-xs text-slate-500 dark:text-gray-400 block">إجمالي الأكواد</span>
					<span class="text-lg font-bold text-blue-500 dark:text-blue-400 font-mono">%d</span>
				</div>
				<div class="bg-slate-100 dark:bg-[#0b0c10] px-4 py-2 rounded-xl border border-green-500/20 text-center">
					<span class="text-xs text-green-600 dark:text-green-400 block">النشطين حالياً</span>
					<span class="text-lg font-bold text-green-500 dark:text-green-400 font-mono">%d</span>
				</div>
				<div class="bg-slate-100 dark:bg-[#0b0c10] px-4 py-2 rounded-xl border border-amber-500/20 text-center">
					<span class="text-xs text-amber-600 dark:text-amber-400 block">ينتهون هذا الأسبوع</span>
					<span class="text-lg font-bold text-amber-500 dark:text-amber-400 font-mono">%d</span>
				</div>
				<div class="bg-slate-100 dark:bg-[#0b0c10] px-4 py-2 rounded-xl border border-red-500/20 text-center">
					<span class="text-xs text-red-600 dark:text-red-400 block">منتهي/معطل/محظور</span>
					<span class="text-lg font-bold text-red-500 dark:text-red-400 font-mono">%d</span>
				</div>
			</header>

			<div class="bg-white dark:bg-[#1f2833] p-5 rounded-2xl border border-slate-200 dark:border-gray-800 mb-6 shadow-xl">
				<h3 class="text-sm font-bold text-blue-500 dark:text-blue-400 mb-3 flex items-center gap-2">✨ توليد وتفعيل فوري</h3>
				<form action="/api/add" method="GET" class="space-y-3">
					<input type="hidden" name="token" value="%s">
					<div class="grid grid-cols-1 sm:grid-cols-4 gap-3">
						<div class="relative flex bg-slate-100 dark:bg-[#0b0c10] rounded-xl border border-slate-300 dark:border-gray-700 overflow-hidden">
							<input type="text" id="code_input" name="code" placeholder="كود الزبون" class="w-full bg-transparent p-3 text-center focus:outline-none font-mono text-blue-500 dark:text-blue-400 font-bold text-sm" required>
							<button type="button" onclick="autoGen()" class="bg-blue-600/10 hover:bg-blue-600 text-blue-500 hover:text-white px-3 text-xs font-bold transition">توليد</button>
						</div>
						<input type="text" name="name" placeholder="اسم الزبون" class="bg-slate-100 dark:bg-[#0b0c10] p-3 rounded-xl border border-slate-300 dark:border-gray-700 text-center focus:outline-none text-sm" required>
						<input type="text" name="phone" placeholder="رقم الهاتف" class="bg-slate-100 dark:bg-[#0b0c10] p-3 rounded-xl border border-slate-300 dark:border-gray-700 text-center focus:outline-none font-mono text-sm">
						<input type="number" id="days_input" name="days" placeholder="عدد الأيام" class="bg-slate-100 dark:bg-[#0b0c10] p-3 rounded-xl border border-slate-300 dark:border-gray-700 text-center focus:outline-none font-mono text-sm" required>
					</div>
					<div class="flex flex-wrap gap-2 items-center justify-between pt-1">
						<div class="flex gap-2">
							<button type="button" onclick="setDays(1)" class="bg-slate-200 hover:bg-slate-300 dark:bg-gray-800 dark:hover:bg-gray-700 text-slate-700 dark:text-gray-300 px-3 py-1.5 rounded-lg text-xs font-semibold">تجربة يوم</button>
							<button type="button" onclick="setDays(30)" class="bg-slate-200 hover:bg-slate-300 dark:bg-gray-800 dark:hover:bg-gray-700 text-slate-700 dark:text-gray-300 px-3 py-1.5 rounded-lg text-xs font-semibold">شهر (30)</button>
							<button type="button" onclick="setDays(180)" class="bg-slate-200 hover:bg-slate-300 dark:bg-gray-800 dark:hover:bg-gray-700 text-slate-700 dark:text-gray-300 px-3 py-1.5 rounded-lg text-xs font-semibold">6 أشهر (180)</button>
							<button type="button" onclick="setDays(365)" class="bg-slate-200 hover:bg-slate-300 dark:bg-gray-800 dark:hover:bg-gray-700 text-slate-700 dark:text-gray-300 px-3 py-1.5 rounded-lg text-xs font-semibold">سنة (365)</button>
						</div>
						<button type="submit" class="w-full sm:w-auto bg-blue-600 hover:bg-blue-700 text-white font-bold px-6 py-2 rounded-xl text-sm transition">حفظ وتشغيل</button>
					</div>
				</form>
			</div>

			<div class="bg-white dark:bg-[#1f2833] rounded-2xl border border-slate-200 dark:border-gray-800 overflow-hidden shadow-2xl mb-6">
				<div class="hidden md:block overflow-x-auto">
					<table class="w-full text-center text-sm">
						<thead class="bg-slate-100 dark:bg-[#141c24] text-slate-500 dark:text-gray-400 border-b border-slate-200 dark:border-gray-800">
							<tr>
								<th class="p-4">كود المشترك</th>
								<th class="p-4">اسم الزبون</th>
								<th class="p-4">تاريخ الانتهاء</th>
								<th class="p-4">الحالة</th>
								<th class="p-4">الإجراءات والواتساب</th>
							</tr>
						</thead>
						<tbody>`, totalCount, activeCount, expiringSoonCount, expiredCount, adminToken)

	dbMutex.RLock()
	for _, sub := range appData.Customers {
		statusStr := "<span class='text-green-600 bg-green-500/10 px-2.5 py-1 rounded-md border border-green-500/20 text-xs font-semibold'>نشط</span>"
		if !sub.IsActive {
			statusStr = "<span class='text-red-500 bg-red-500/10 px-2.5 py-1 rounded-md border border-red-500/30 text-xs font-bold'>🚫 محظور/معطل</span>"
		} else if time.Now().After(sub.ExpireTime) {
			statusStr = "<span class='text-red-600 bg-red-500/10 px-2.5 py-1 rounded-md border border-red-500/20 text-xs font-semibold'>منتهي</span>"
		} else if time.Until(sub.ExpireTime) < 7*24*time.Hour {
			statusStr = "<span class='text-amber-600 bg-amber-500/10 px-2.5 py-1 rounded-md border border-amber-500/20 text-xs font-semibold'>قريب الانتهاء</span>"
		}

		formattedExpiry := sub.ExpireTime.Format("2006-01-02 15:04")

		fmt.Fprintf(w, `
							<tr class="border-b border-slate-100 dark:border-gray-800/40 hover:bg-slate-50 dark:hover:bg-[#141c24]/30 transition">
								<td class="p-4 font-mono text-blue-500 dark:text-blue-400 font-bold tracking-wider">%s</td>
								<td class="p-4 text-slate-800 dark:text-gray-200 font-medium">%s</td>
								<td class="p-4 font-mono text-slate-500 dark:text-gray-400 text-xs">%s</td>
								<td class="p-4">%s</td>
								<td class="p-4 flex justify-center gap-1.5">
									<button onclick="generateCardImage('%s', '%s', '%s', '%s')" class="bg-amber-500/20 text-amber-600 border border-amber-500/40 px-2.5 py-1.5 rounded-xl text-xs hover:bg-amber-500 hover:text-white transition font-bold">كارت كصورة 📸</button>
									<button onclick="sendWhatsApp('%s', '%s', '%s', '%s')" class="bg-green-600/20 text-green-600 dark:text-green-400 border border-green-600/30 px-2.5 py-1.5 rounded-xl text-xs hover:bg-green-600 hover:text-white transition font-bold">واتساب 💬</button>
									<button onclick="copyActivation('%s', '%s', '%s')" class="bg-blue-500/10 text-blue-500 dark:text-blue-400 border border-blue-500/20 px-2 py-1.5 rounded-xl text-xs hover:bg-blue-500 hover:text-white transition">نسخ</button>
									<a href="/api/toggle?token=%s&code=%s" class="bg-slate-500/20 text-slate-600 dark:text-slate-400 border border-slate-500/30 px-2 py-1.5 rounded-xl text-xs font-bold">قفل/فتح 🔐</a>
									<a href="/api/renew?token=%s&code=%s" class="bg-purple-600/20 text-purple-600 dark:text-purple-400 border border-purple-600/30 px-2 py-1.5 rounded-xl text-xs hover:bg-purple-600 hover:text-white transition font-bold">تجديد سنة ⏳</a>
									<a href="/api/delete?token=%s&code=%s" class="bg-red-500/10 text-red-600 border border-red-500/20 px-2 py-1.5 rounded-xl text-xs hover:bg-red-600 hover:text-white">حذف</a>
								</td>
							</tr>`, sub.Code, sub.Name, formattedExpiry, statusStr, sub.Name, sub.Code, currentHost, formattedExpiry, sub.Phone, sub.Name, sub.Code, currentHost, sub.Name, sub.Code, currentHost, adminToken, sub.Code, adminToken, sub.Code, adminToken, sub.Code)
	}
	dbMutex.RUnlock()

	fmt.Fprintf(w, `
						</tbody>
					</table>
				</div>

				<div class="block md:hidden p-4 space-y-4">`)

	dbMutex.RLock()
	for _, sub := range appData.Customers {
		statusStr := "<span class='text-green-600 bg-green-500/10 px-2 py-0.5 rounded border border-green-500/20 text-xs'>نشط</span>"
		if !sub.IsActive {
			statusStr = "<span class='text-red-500 bg-red-500/10 px-2 py-0.5 rounded border border-red-500/20 text-xs font-bold'>🚫 محظور</span>"
		} else if time.Now().After(sub.ExpireTime) {
			statusStr = "<span class='text-red-600 bg-red-500/10 px-2 py-0.5 rounded border border-red-500/20 text-xs'>منتهي</span>"
		} else if time.Until(sub.ExpireTime) < 7*24*time.Hour {
			statusStr = "<span class='text-amber-600 bg-amber-500/10 px-2 py-0.5 rounded border border-amber-500/20 text-xs'>ينتهي قريباً</span>"
		}
		formattedExpiry := sub.ExpireTime.Format("2006-01-02 15:04")

		fmt.Fprintf(w, `
					<div class="bg-slate-50 dark:bg-[#0b0c10] p-4 rounded-xl border border-slate-200 dark:border-gray-800 space-y-2">
						<div class="flex justify-between items-center">
							<span class="font-mono text-blue-500 dark:text-blue-400 font-bold text-base">%s</span>
							%s
						</div>
						<div class="text-xs text-slate-700 dark:text-gray-300">الاسم: <span class="text-slate-900 dark:text-gray-400">%s</span></div>
						<div class="text-[11px] text-slate-500 dark:text-gray-500 font-mono">ينتهي: %s</div>
						<div class="grid grid-cols-2 gap-1.5 pt-2 border-t border-slate-200 dark:border-gray-800/60">
							<button onclick="generateCardImage('%s', '%s', '%s', '%s')" class="text-center bg-amber-500/20 text-amber-600 border border-amber-500/30 py-1 rounded-lg text-xs font-bold">صورة 📸</button>
							<button onclick="sendWhatsApp('%s', '%s', '%s', '%s')" class="text-center bg-green-600/20 text-green-600 dark:text-green-400 border border-green-600/30 py-1 rounded-lg text-xs font-bold">واتساب</button>
							<a href="/api/toggle?token=%s&code=%s" class="text-center bg-slate-500/20 text-slate-400 border border-slate-500/30 py-1 rounded-lg text-xs font-bold">قفل/فتح</a>
							<a href="/api/renew?token=%s&code=%s" class="text-center bg-purple-600/20 text-purple-600 dark:text-purple-400 border border-purple-600/30 py-1 rounded-lg text-xs font-bold">تجديد</a>
						</div>
					</div>`, sub.Code, statusStr, sub.Name, formattedExpiry, sub.Name, sub.Code, currentHost, formattedExpiry, sub.Phone, sub.Name, sub.Code, currentHost, adminToken, sub.Code, adminToken, sub.Code)
	}
	dbMutex.RUnlock()

	fmt.Fprintf(w, `
				</div>
			</div>

			<div class="bg-white dark:bg-[#1f2833] p-5 rounded-2xl border border-slate-200 dark:border-gray-800 shadow-xl space-y-6">
				<h3 class="text-sm font-bold text-amber-500 flex items-center gap-2">⚙️ مصفوفة دمج السيرفرات والتحويل الآلي الفوري</h3>
				<form action="/api/update-servers" method="GET" class="space-y-4">
					<input type="hidden" name="token" value="%s">
					
					<div class="p-4 bg-slate-50 dark:bg-[#0b0c10] rounded-xl border border-blue-500/20 grid grid-cols-1 sm:grid-cols-3 gap-3">
						<div class="sm:col-span-3 text-xs font-bold text-blue-500 dark:text-blue-400">السيرفر الرئيسي الأول (Server 1)</div>
						<input type="text" name="s1_url" value="%s" placeholder="رابط السيرفر الأول" class="bg-white dark:bg-[#1f2833] p-2.5 rounded-lg border border-slate-300 dark:border-gray-700 text-xs font-mono focus:outline-none">
						<input type="text" name="s1_user" value="%s" placeholder="اسم المستخدم" class="bg-white dark:bg-[#1f2833] p-2.5 rounded-lg border border-slate-300 dark:border-gray-700 text-xs font-mono focus:outline-none">
						<input type="text" name="s1_pass" value="%s" placeholder="كلمة المرور" class="bg-white dark:bg-[#1f2833] p-2.5 rounded-lg border border-slate-300 dark:border-gray-700 text-xs font-mono focus:outline-none">
					</div>

					<div class="p-4 bg-slate-50 dark:bg-[#0b0c10] rounded-xl border border-amber-500/10 grid grid-cols-1 sm:grid-cols-3 gap-3">
						<div class="sm:col-span-3 text-xs font-bold text-amber-500">السيرفر الاحتياطي الثاني (Server 2)</div>
						<input type="text" name="s2_url" value="%s" placeholder="رابط السيرفر الثاني" class="bg-white dark:bg-[#1f2833] p-2.5 rounded-lg border border-slate-300 dark:border-gray-700 text-xs font-mono focus:outline-none">
						<input type="text" name="s2_user" value="%s" placeholder="اسم المستخدم" class="bg-white dark:bg-[#1f2833] p-2.5 rounded-lg border border-slate-300 dark:border-gray-700 text-xs font-mono focus:outline-none">
						<input type="text" name="s2_pass" value="%s" placeholder="كلمة المرور" class="bg-white dark:bg-[#1f2833] p-2.5 rounded-lg border border-slate-300 dark:border-gray-700 text-xs font-mono focus:outline-none">
					</div>

					<div class="p-4 bg-slate-50 dark:bg-[#0b0c10] rounded-xl border border-red-500/10 grid grid-cols-1 sm:grid-cols-3 gap-3">
						<div class="sm:col-span-3 text-xs font-bold text-red-500 dark:text-red-400">السيرفر الاحتياطي الثالث (Server 3)</div>
						<input type="text" name="s3_url" value="%s" placeholder="رابط السيرفر الثالث" class="bg-white dark:bg-[#1f2833] p-2.5 rounded-lg border border-slate-300 dark:border-gray-700 text-xs font-mono focus:outline-none">
						<input type="text" name="s3_user" value="%s" placeholder="اسم المستخدم" class="bg-white dark:bg-[#1f2833] p-2.5 rounded-lg border border-slate-300 dark:border-gray-700 text-xs font-mono focus:outline-none">
						<input type="text" name="s3_pass" value="%s" placeholder="كلمة المرور" class="bg-white dark:bg-[#1f2833] p-2.5 rounded-lg border border-slate-300 dark:border-gray-700 text-xs font-mono focus:outline-none">
					</div>

					<button type="submit" class="w-full bg-amber-600 hover:bg-amber-700 text-white font-bold py-3 rounded-xl text-sm transition">حفظ مصفوفة السيرفرات</button>
				</form>
			</div>

		</div>
	</body>
	</html>`, adminToken,
		cfg.Server1.URL, cfg.Server1.User, cfg.Server1.Pass,
		cfg.Server2.URL, cfg.Server2.User, cfg.Server2.Pass,
		cfg.Server3.URL, cfg.Server3.User, cfg.Server3.Pass)
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
		phone := r.URL.Query().Get("phone")
		var days int
		fmt.Sscanf(r.URL.Query().Get("days"), "%d", &days)

		if code != "" && name != "" && days > 0 {
			appData.Customers[code] = Subscriber{
				Code:       code,
				Name:       name,
				Phone:      phone,
				ExpireTime: time.Now().Add(time.Duration(days) * 24 * time.Hour),
				IsActive:   true,
			}
			saveDB()
		}
	} else if path == "/api/renew" {
		code := r.URL.Query().Get("code")
		if sub, exists := appData.Customers[code]; exists {
			if time.Now().After(sub.ExpireTime) {
				sub.ExpireTime = time.Now().Add(365 * 24 * time.Hour)
			} else {
				sub.ExpireTime = sub.ExpireTime.Add(365 * 24 * time.Hour)
			}
			sub.IsActive = true
			appData.Customers[code] = sub
			saveDB()
		}
	} else if path == "/api/delete" {
		code := r.URL.Query().Get("code")
		delete(appData.Customers, code)
		saveDB()
	} else if path == "/api/toggle" {
		code := r.URL.Query().Get("code")
		if sub, exists := appData.Customers[code]; exists {
			sub.IsActive = !sub.IsActive
			if sub.IsActive {
				sub.LastIP = ""
				sub.LastUA = ""
			}
			appData.Customers[code] = sub
			saveDB()
		}
	} else if path == "/api/update-servers" {
		appData.Config.Server1 = ServerCreds{URL: r.URL.Query().Get("s1_url"), User: r.URL.Query().Get("s1_user"), Pass: r.URL.Query().Get("s1_pass")}
		appData.Config.Server2 = ServerCreds{URL: r.URL.Query().Get("s2_url"), User: r.URL.Query().Get("s2_user"), Pass: r.URL.Query().Get("s2_pass")}
		appData.Config.Server3 = ServerCreds{URL: r.URL.Query().Get("s3_url"), User: r.URL.Query().Get("s3_user"), Pass: r.URL.Query().Get("s3_pass")}
		saveDB()
	}

	dbMutex.Unlock()
	http.Redirect(w, r, "/dashboard?token="+adminToken, http.StatusSeeOther)
}

func handleIPTVRedirect(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	password := r.URL.Query().Get("password")
	action := r.URL.Query().Get("action")

	dbMutex.Lock()
	sub, exists := appData.Customers[username]
	cfg := appData.Config

	if !exists || password != username || !sub.IsActive || time.Now().After(sub.ExpireTime) {
		dbMutex.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"user_info":{"auth":0,"status":"Account Disabled or Expired"}}`))
		return
	}

	currentIP := getIP(r)
	currentUA := r.Header.Get("User-Agent")

	if sub.LastIP != "" && sub.LastUA != "" {
		if time.Since(sub.LastSeen) < 5*time.Minute {
			if sub.LastIP != currentIP || sub.LastUA != currentUA {
				sub.IsActive = false
				appData.Customers[username] = sub
				saveDB()
				dbMutex.Unlock()

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"user_info":{"auth":0,"status":"Banned: Multi-Device Detected"}}`))
				return
			}
		}
	}

	sub.LastIP = currentIP
	sub.LastUA = currentUA
	sub.LastSeen = time.Now()
	appData.Customers[username] = sub
	saveDB()
	dbMutex.Unlock()

	var target ServerCreds
	scData.Mutex.Lock()
	s1Alive := scData.ServerStatus[cfg.Server1.URL]
	s2Alive := scData.ServerStatus[cfg.Server2.URL]
	s3Alive := scData.ServerStatus[cfg.Server3.URL]
	scData.Mutex.Unlock()

	if s1Alive {
		target = cfg.Server1
	} else if s2Alive {
		target = cfg.Server2
	} else if s3Alive {
		target = cfg.Server3
	} else {
		target = cfg.Server1
	}

	var redirectURL string
	if action != "" {
		redirectURL = fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=%s", target.URL, target.User, target.Pass, action)
	} else {
		redirectURL = fmt.Sprintf("%s/get.php?username=%s&password=%s&output=ts", target.URL, target.User, target.Pass)
	}

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

func main() {
	http.HandleFunc("/player_api.php", handleIPTVRedirect)
	http.HandleFunc("/get.php", handleIPTVRedirect)
	http.HandleFunc("/dashboard", handleDashboard)
	http.HandleFunc("/api/add", handleAPI)
	http.HandleFunc("/api/renew", handleAPI)
	http.HandleFunc("/api/delete", handleAPI)
	http.HandleFunc("/api/toggle", handleAPI)
	http.HandleFunc("/api/update-servers", handleAPI)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Server running on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
