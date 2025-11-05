package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	// 1. 导入 act 的 model 包
	"github.com/nektos/act/pkg/model"
	"gopkg.in/yaml.v3"
)

// #############################################################################
// 4. Goroutine "线程池" (Worker Pool) 实现
// #############################################################################

const (
	MaxWorkers = 5   // 池中最大 "worker" (goroutine) 数量
	MaxQueue   = 100 // 任务队列的最大容量
)

// ConversionJob 定义了我们的 "任务"
// 它包含了需要处理的数据，以及一个用于回传结果的 channel
type ConversionJob struct {
	Payload    []byte                // 原始的 YAML 数据
	ResultChan chan ConversionResult // 用于回传结果的通道
}

// ConversionResult 定义了任务处理的结果
type ConversionResult struct {
	Data  string // 转换后的数据（这里是 JSON 字符串）
	Error error  // 处理过程中发生的错误
}

// JobQueue 是一个全局的任务队列（带缓冲的 channel）
// Web API 处理器将向这个队列发送任务
var JobQueue chan ConversionJob

// StartWorkerPool 初始化并启动我们的 worker 池
func StartWorkerPool() {
	// 1. 初始化 JobQueue
	JobQueue = make(chan ConversionJob, MaxQueue)

	// 2. 启动指定数量的 worker goroutine
	for i := 1; i <= MaxWorkers; i++ {
		go func(workerID int) {
			log.Printf("Worker %d 启动", workerID)

			// 3. Worker 循环地从 JobQueue 中读取任务
			for job := range JobQueue {
				log.Printf("Worker %d 开始处理任务", workerID)

				// 4. 执行 "转换" 逻辑
				convertedData, err := convertWorkflow(job.Payload)

				// 5. 将结果通过 ResultChan 发送回给提交者 (HTTP 处理器)
				job.ResultChan <- ConversionResult{
					Data:  convertedData,
					Error: err,
				}
			}
		}(i)
	}
}

// #############################################################################
// 2. Workflow 转换逻辑
// #############################################################################

// SimplifiedWorkflow 是我们“转换”后的目标结构
type SimplifiedWorkflow struct {
	Name string   `json:"name"`
	Jobs []string `json:"jobs"`
}

// convertWorkflow 是核心的转换函数
func convertWorkflow(yamlData []byte) (string, error) {
	var workflow model.Workflow

	// 1. 使用 gopkg.in/yaml.v3 将 YAML 字节流解析到 act 的 model.Workflow 结构体中
	err := yaml.Unmarshal(yamlData, &workflow)
	if err != nil {
		return "", fmt.Errorf("解析 YAML 失败: %w", err)
	}

	// 2. "转换" 逻辑：提取 Job ID 列表
	// 你可以在这里实现你自己的复杂转换逻辑，例如转为 GitLab CI 或 Jenkinsfile 格式
	jobIDs := make([]string, 0, len(workflow.Jobs))
	for jobID := range workflow.Jobs {
		jobIDs = append(jobIDs, jobID)
	}

	// 3. 创建简化的结构体
	simplified := SimplifiedWorkflow{
		Name: workflow.Name,
		Jobs: jobIDs,
	}

	// 4. 将简化后的结构体序列化为 JSON 字符串
	jsonData, err := json.Marshal(simplified)
	if err != nil {
		return "", fmt.Errorf("序列化 JSON 失败: %w", err)
	}

	return string(jsonData), nil
}

// #############################################################################
// 3. Golang Web 服务
// #############################################################################

// handleConversion 是我们的 API 处理器
func handleConversion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST 方法", http.StatusMethodNotAllowed)
		return
	}

	// 1. 读取请求体 (YAML)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if len(body) == 0 {
		http.Error(w, "请求体为空", http.StatusBadRequest)
		return
	}

	// 2. 创建一个用于接收此特定请求结果的 channel
	resultChan := make(chan ConversionResult)

	// 3. 创建一个新任务
	job := ConversionJob{
		Payload:    body,
		ResultChan: resultChan,
	}

	// 4. 将任务发送到 "线程池" 的任务队列
	select {
	case JobQueue <- job:
		log.Println("任务已提交到队列")
	default:
		// 如果 JobQueue 已满，立即返回错误
		http.Error(w, "服务繁忙，任务队列已满", http.StatusServiceUnavailable)
		return
	}

	// 5. 等待 Worker 完成任务并回传结果
	// 注意：这里 HTTP 处理器会阻塞，直到 worker 处理完毕
	// 这是一种同步的 API 风格，但后端处理是并发池化的
	log.Println("等待任务结果...")
	result := <-resultChan

	// 6. 处理结果
	if result.Error != nil {
		log.Printf("任务处理失败: %v", result.Error)
		http.Error(w, fmt.Sprintf("转换失败: %s", result.Error.Error()), http.StatusInternalServerError)
		return
	}

	// 7. 成功，返回转换后的 JSON
	log.Println("任务处理成功")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(result.Data))
}

func main() {
	// 启动 Worker 池
	StartWorkerPool()
	log.Println("Worker 池已启动")

	// 注册 API 路由
	http.HandleFunc("/api/v1/convert", handleConversion)

	// 启动 Web 服务
	log.Println("Web 服务启动于 http://localhost:8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("服务启动失败: ", err)
	}
}
