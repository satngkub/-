package src

import (
	"fmt"
	"github.com/fatih/color"
	"gopkg.in/go-playground/validator.v9"
	"io"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"time"
)

// CC คือโครงสร้างหลักสำหรับควบคุมการทดสอบโหลด
type CC struct {
	Url     string `validate:"url,required"`
	Worker  int    `validate:"required,gt=0"` // Worker ต้องมากกว่า 0
	Time    int    `validate:"required,gt=0"` // Time ต้องมากกว่า 0
	*Scheduler
	*Report
	*useragent
	httpClient *http.Client // เพิ่ม HttpClient สำหรับ Reusing Connection
}

// Scheduler จัดการสัญญาณในการเริ่มและหยุด Worker
type Scheduler struct {
	Signal *chan bool
	Done   func()
}

// Report เก็บข้อมูลการวัดผลลัพธ์
type Report struct {
	TotalRequests int32 // จำนวน Request ทั้งหมดที่ส่งออกไป
	ErrorCount    int32 // จำนวน Error ที่เกิดขึ้น
	SuccessCount  int32 // จำนวน Request ที่สำเร็จ
}

// New เตรียมความพร้อมสำหรับการทดสอบโหลด
func (c *CC) New() error {
	c.Report = new(Report)
	c.Scheduler = new(Scheduler)
	c.useragent = newUA()

	// ตั้งค่า http.Client สำหรับการ Load Test
	// นี่คือส่วนสำคัญที่ช่วยให้ประสิทธิภาพสูงขึ้น
	tr := &http.Transport{
		MaxIdleConns:        100000,          // จำนวน Connection ที่ไม่ได้ใช้งานสูงสุดใน Pool
		MaxIdleConnsPerHost: 10000,           // จำนวน Connection ที่ไม่ได้ใช้งานต่อ Host สูงสุด
		IdleConnTimeout:     90 * time.Second, // ระยะเวลาที่ Connection ที่ไม่ได้ใช้งานจะถูกปิด
		DisableKeepAlives:   false,            // เปิดใช้งาน Keep-Alive โดยค่าเริ่มต้น
		// TLSHandshakeTimeout: 10 * time.Second, // สามารถ uncomment และปรับแต่งได้ตามต้องการ
		// ExpectContinueTimeout: 1 * time.Second,
	}
	c.httpClient = &http.Client{
		Transport: tr,
		Timeout:   5 * time.Second, // ตั้งค่า Timeout เพื่อป้องกัน Request ค้าง
	}

	ch := make(chan bool)
	c.Scheduler.Signal = &ch
	c.Scheduler.Done = c.ShutDown()

	v := validator.New()
	err := v.Struct(c)
	return err
}

// Start เริ่มต้นกระบวนการทดสอบโหลด
func (c *CC) Start() {
	d := color.New(color.FgCyan, color.Bold)

	// Goroutine สำหรับแสดงผลสถานะแบบ Real-time
	// จะอัปเดตทุก 1 วินาที แต่ไม่ขัดขวาง Worker
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		lastTotalRequests := int32(0)
		startTime := time.Now()

		for {
			select {
			case <-(*c.Scheduler.Signal): // รับสัญญาณ shutdown
				return
			case <-ticker.C:
				currentTotalRequests := atomic.LoadInt32(&c.Report.TotalRequests)
				currentErrorCount := atomic.LoadInt32(&c.Report.ErrorCount)
				currentSuccessCount := atomic.LoadInt32(&c.Report.SuccessCount)

				// คำนวณ RPS (Requests Per Second) สำหรับวินาทีที่ผ่านมา
				requestsInLastSecond := currentTotalRequests - lastTotalRequests
				lastTotalRequests = currentTotalRequests

				// คำนวณเวลาที่ผ่านไปทั้งหมด (เป็นวินาที)
				elapsedTime := time.Since(startTime).Seconds()

				// แสดงผลลัพธ์
				d.Printf("\rrolling:  workers: %d | time: %ds | RPS: %d/s | Total: %d | Success: %d | Errors: %d ",
					c.Worker,
					int(elapsedTime),
					requestsInLastSecond, // RPS สำหรับวินาทีที่ผ่านมา
					currentTotalRequests,
					currentSuccessCount,
					currentErrorCount,
				)
			}
		}
	}()

	// เริ่ม Worker Goroutine ตามจำนวนที่กำหนด
	for i := 0; i < c.Worker; i++ {
		go func() {
			for {
				select {
				case <-(*c.Scheduler.Signal): // รับสัญญาณ shutdown
					return
				default:
					// ยิง Request ได้ทันที ไม่มีการหน่วง
					resp, err := c.get(c.Url, c.useragent.random()) // ใช้ c.get เพื่อเข้าถึง httpClient
					atomic.AddInt32(&c.Report.TotalRequests, 1)    // นับ Request ที่ส่งไป

					if err == nil {
						atomic.AddInt32(&c.Report.SuccessCount, 1)
						// สำคัญ: ต้องอ่าน Body และปิด Body เพื่อให้ Connection ถูกปล่อยกลับเข้า Pool
						_, _ = io.Copy(ioutil.Discard, resp.Body)
						_ = resp.Body.Close()
					} else {
						atomic.AddInt32(&c.Report.ErrorCount, 1)
						// ไม่ควร Print Error ทุกครั้งใน Load Test ที่มีปริมาณสูงมาก
						// เพราะจะทำให้ IO ช้าและ Console รก
						// fmt.Println("Request Error:", err)
					}
				}
			}
		}()
	}

	// รอให้โปรแกรมรันครบตามระยะเวลาที่กำหนด
	time.Sleep(time.Duration(c.Time) * time.Second)
}

// ShutDown ส่งสัญญาณให้ Worker ทั้งหมดหยุดทำงาน
func (c *CC) ShutDown() func() {
	return func() {
		// ส่งสัญญาณ shutdown ไปยัง Worker ทุกตัว
		for i := 0; i < c.Worker; i++ {
			*c.Scheduler.Signal <- true
		}
		// ปิด Channel หลังจากส่งสัญญาณครบ
		close(*c.Scheduler.Signal)
		fmt.Println("\nShutting down workers...")
	}
}

// get ส่ง HTTP GET Request โดยใช้ httpClient ที่สร้างไว้
func (c *CC) get(url, useragent string) (*http.Response, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Add("User-Agent", useragent)

	resp, err := c.httpClient.Do(request) // ใช้ httpClient ของ struct CC

	return resp, err
}

// post ยังไม่ได้ implement สำหรับการส่ง POST Request
func post(url, useragent string) {
	// สามารถเพิ่มโค้ดสำหรับ POST Request ที่นี่ได้ในภายหลัง
}

// useragent related structs and functions
type useragent struct {
	list []string
}

func newUA() *useragent {
	return &useragent{
		list: []string{
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36",
			"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.1 Safari/605.1.15",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36 OPR/95.0.0.0",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36 EdgA/109.0.1518.78",
			"Mozilla/5.0 (iPhone; CPU iPhone OS 16_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/109.0.5414.85 Mobile/15E148 Safari/604.1",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/109.0",
		},
	}
}

func (u *useragent) random() string {
	return u.list[time.Now().UnixNano()%int64(len(u.list))]
}

