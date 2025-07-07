// main.go
package main

import (
	"fmt"
	"log"
	"os" // import package os เพื่อเข้าถึง Command Line Arguments
	"strconv" // import package strconv สำหรับแปลง string เป็น int
	"cc-go-attack/src" // เปลี่ยนตาม Path ของ package src ของคุณ
)

func main() {
	// ตรวจสอบจำนวน arguments ที่ส่งมา
	// os.Args[0] คือชื่อโปรแกรม (main.go)
	// os.Args[1] คือ URL
	// os.Args[2] คือ Worker (ถ้ามี)
	// os.Args[3] คือ Time (ถ้ามี)

	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <URL> [workers] [time_in_seconds]")
		fmt.Println("Example: go run main.go https://www.example.com 50000 60")
		os.Exit(1) // ออกจากโปรแกรมพร้อมรหัสข้อผิดพลาด
	}

	url := os.Args[1] // ดึง URL จาก Argument แรก

	// กำหนดค่าเริ่มต้นสำหรับ Worker และ Time
	// คุณสามารถปรับค่าเริ่มต้นเหล่านี้ได้ตามความเหมาะสม
	defaultWorkers := 50000
	defaultTime := 60

	// ตรวจสอบและดึงค่า Worker จาก Argument ที่สอง (ถ้ามี)
	workers := defaultWorkers
	if len(os.Args) > 2 {
		parsedWorkers, err := strconv.Atoi(os.Args[2])
		if err != nil {
			log.Printf("Warning: Invalid workers value '%s'. Using default %d workers.\n", os.Args[2], defaultWorkers)
		} else {
			workers = parsedWorkers
		}
	}

	// ตรวจสอบและดึงค่า Time จาก Argument ที่สาม (ถ้ามี)
	timeInSeconds := defaultTime
	if len(os.Args) > 3 {
		parsedTime, err := strconv.Atoi(os.Args[3])
		if err != nil {
			log.Printf("Warning: Invalid time value '%s'. Using default %d seconds.\n", os.Args[3], defaultTime)
		} else {
			timeInSeconds = parsedTime
		}
	}


	loadTester := &src.CC{
		Url:    url, // ใช้ URL ที่ดึงมาจาก Command Line Argument
		Worker: workers,
		Time:   timeInSeconds,
	}

	err := loadTester.New()
	if err != nil {
		log.Fatalf("Failed to initialize load tester: %v", err)
	}

	fmt.Println("Starting load test...")
	fmt.Printf("Target URL: %s\n", loadTester.Url)
	fmt.Printf("Workers: %d\n", loadTester.Worker)
	fmt.Printf("Duration: %d seconds\n", loadTester.Time)
	fmt.Println("--------------------------------------------------")

	loadTester.Start()

	loadTester.Done() // เรียก ShutDown เมื่อโปรแกรมรันครบเวลา
	fmt.Println("\nLoad test finished.")
	fmt.Printf("--------------------------------------------------\n")
	fmt.Printf("Summary:\n")
	fmt.Printf("  Total Requests Sent: %d\n", loadTester.Report.TotalRequests)
	fmt.Printf("  Successful Requests: %d\n", loadTester.Report.SuccessCount)
	fmt.Printf("  Failed Requests:     %d\n", loadTester.Report.ErrorCount)

	if loadTester.Report.TotalRequests > 0 {
		fmt.Printf("  Overall RPS (Average): %.2f req/s\n", float64(loadTester.Report.TotalRequests)/float64(loadTester.Time))
	}
}

