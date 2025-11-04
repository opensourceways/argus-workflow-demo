package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"

	// 1. Argo Workflow API 结构体
	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	// 2. nektos/act GHA 解析器
	"github.com/nektos/act/pkg/model"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- 线程池（Worker Pool）配置 ---

const (
	NumWorkers = 5   // 工作 Goroutine 的数量
	MaxQueue   = 100 // 作业队列的最大缓冲
)

// ConversionJob 定义了需要传递给 worker 的作业
type ConversionJob struct {
	JobID   string // 唯一的作业 ID
	GhaYAML string // 输入的 GHA YAML
}

// ConversionResult 定义了 worker 的处理结果
type ConversionResult struct {
	JobID    string // 原始作业 ID
	ArgoYAML string // 输出的 Argo YAML
	Error    error  // 处理过程中发生的错误
}

// 全局变量：作业队列和结果存储
var JobQueue chan ConversionJob
var ResultStore *sync.Map // 使用 sync.Map 保证并发安全

// --- Web 服务入口 (main) ---

func main() {
	// 1. 初始化作业队列和结果存储
	JobQueue = make(chan ConversionJob, MaxQueue)
	ResultStore = &sync.Map{}

	// 2. 启动线程池
	log.Printf("Starting %d workers...", NumWorkers)
	startWorkerPool(NumWorkers, JobQueue, ResultStore)

	// 3. 设置 HTTP 路由
	http.HandleFunc("/convert", handleConvert)
	http.HandleFunc("/result/", handleGetResult)

	// 4. 启动 Web 服务
	log.Println("Starting server on :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

// --- 线程池实现 ---

// startWorkerPool 启动指定数量的 worker goroutine
func startWorkerPool(numWorkers int, jobQueue <-chan ConversionJob, resultStore *sync.Map) {
	for i := 1; i <= numWorkers; i++ {
		go func(workerID int) {
			log.Printf("Worker %d started", workerID)
			// 从作业队列中循环读取作业
			for job := range jobQueue {
				log.Printf("Worker %d processing job %s", workerID, job.JobID)

				// 执行核心转换逻辑
				argoWF, err := convertGHAtoArgo(job.GhaYAML)
				result := ConversionResult{JobID: job.JobID}

				if err != nil {
					log.Printf("Worker %d failed job %s: %v", workerID, job.JobID, err)
					result.Error = err
				} else {
					// 将 Argo 结构体序列化为 YAML 字符串
					yamlBytes, marshalErr := yaml.Marshal(argoWF)
					if marshalErr != nil {
						result.Error = fmt.Errorf("failed to marshal Argo YAML: %v", marshalErr)
					} else {
						result.ArgoYAML = string(yamlBytes)
						log.Printf("Worker %d completed job %s", workerID, job.JobID)
					}
				}

				// 将结果存入 sync.Map
				resultStore.Store(job.JobID, result)
			}
		}(i)
	}
}

// --- HTTP 处理器 ---

// handleConvert (POST /convert) 接收 GHA YAML 并分发作业
func handleConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	if len(body) == 0 {
		http.Error(w, "Request body is empty", http.StatusBadRequest)
		return
	}

	// 1. 创建新作业
	jobID := uuid.New().String()
	job := ConversionJob{
		JobID:   jobID,
		GhaYAML: string(body),
	}

	// 2. 尝试将作业发送到队列
	select {
	case JobQueue <- job:
		// 3. 成功分发，返回 202 Accepted
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"status":    "processing",
			"jobID":     jobID,
			"resultURL": fmt.Sprintf("/result/%s", jobID),
		})
	default:
		// 4. 队列已满，返回 503
		http.Error(w, "Server busy, queue is full", http.StatusServiceUnavailable)
	}
}

// handleGetResult (GET /result/{jobID}) 查询作业结果
func handleGetResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method is allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := strings.TrimPrefix(r.URL.Path, "/result/")
	if jobID == "" {
		http.Error(w, "Job ID is missing", http.StatusBadRequest)
		return
	}

	// 1. 从 sync.Map 中加载结果
	result, ok := ResultStore.Load(jobID)
	if !ok {
		// 结果尚未准备好
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "not_found",
			"message": "Job not found or still processing",
		})
		return
	}

	// 2. 转换结果类型
	res := result.(ConversionResult)

	// 3. 检查处理是否出错
	if res.Error != nil {
		http.Error(w, fmt.Sprintf("Failed to process job: %v", res.Error), http.StatusInternalServerError)
		return
	}

	// 4. 返回成功的 YAML 结果
	w.Header().Set("Content-Type", "application/x-yaml")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(res.ArgoYAML))
}

// --- 核心转换逻辑 ---

// convertGHAtoArgo 使用 nektos/act 解析器执行转换
func convertGHAtoArgo(ghaYAML string) (*wfv1.Workflow, error) {
	// 1. 使用 nektos/act/pkg/model 解析 GHA YAML
	ghaReader := strings.NewReader(ghaYAML)
	ghaWF, err := model.ReadWorkflow(ghaReader, false) // 添加第二个参数 false
	if err != nil {
		return nil, fmt.Errorf("failed to parse GHA YAML using 'act': %v", err)
	}

	// 2. 创建 Argo Workflow 基础结构
	argoWF := &wfv1.Workflow{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "Workflow",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: sanitizeName(ghaWF.Name) + "-",
		},
		Spec: wfv1.WorkflowSpec{
			Templates: []wfv1.Template{},
		},
	}

	// 3. 编排 Job (GHA Job -> Argo DAG Task)
	var jobNames []string
	jobTemplates := make(map[string]wfv1.Template)
	jobDependencies := make(map[string][]string)

	for jobName, ghaJob := range ghaWF.Jobs {
		jobTemplateName := sanitizeName(jobName)
		jobNames = append(jobNames, jobTemplateName)

		// 修复：调用 Needs() 方法而不是直接访问字段
		needs := ghaJob.Needs()
		jobDependencies[jobTemplateName] = needs // 记录依赖

		// 为 GHA Job 创建一个 Argo "steps" 模板
		jobTemplate := wfv1.Template{
			Name:  jobTemplateName,
			Steps: []wfv1.ParallelSteps{}, // 修复：使用正确的类型
		}

		// GHA 步骤 -> Argo 模板
		var stepTemplates []wfv1.Template

		for i, ghaStep := range ghaJob.Steps {
			stepName := sanitizeName(ghaStep.Name)
			if stepName == "" {
				stepName = fmt.Sprintf("step-%d", i)
			}
			stepTemplateName := fmt.Sprintf("%s-%s", jobTemplateName, stepName)

			// a. 将 GHA step 添加到 Job 的 "steps" 序列中
			jobTemplate.Steps = append(jobTemplate.Steps, wfv1.ParallelSteps{
				{
					Name:     stepName,
					Template: stepTemplateName,
				},
			})

			// b. 创建 GHA step 对应的 Argo Template
			// 修复：调用 RunsOn() 方法并获取第一个运行环境
			runsOn := ghaJob.RunsOn()
			var baseImage string
			if len(runsOn) > 0 {
				baseImage = mapRunsOnToImage(runsOn[0])
			} else {
				baseImage = "alpine:latest"
			}

			stepTemplate := wfv1.Template{
				Name: stepTemplateName,
			}

			if ghaStep.Run != "" {
				// 转换 GHA 'run' -> Argo 'script'
				stepTemplate.Script = &wfv1.ScriptTemplate{
					Container: corev1.Container{ // 修复：使用 corev1.Container
						Image:   baseImage,
						Command: []string{"bash", "-c"}, // GHA 默认使用 bash
					},
					Source: ghaStep.Run,
				}
			} else if ghaStep.Uses != "" {
				// 转换 GHA 'uses' -> 占位符 (Placeholder)
				withParams := ""
				if ghaStep.With != nil {
					withParams = fmt.Sprintf("Parameters (with): %v", ghaStep.With)
				}

				stepTemplate.Script = &wfv1.ScriptTemplate{
					Container: corev1.Container{ // 修复：使用 corev1.Container
						Image:   "alpine:latest",
						Command: []string{"sh", "-c"},
					},
					Source: fmt.Sprintf(`
echo "****************************************************************"
echo "TODO: Manually implement GHA Action: %s"
echo "%s"
echo "****************************************************************"
exit 1
`, ghaStep.Uses, withParams),
				}
			} else {
				// 跳过空步骤
				continue
			}
			stepTemplates = append(stepTemplates, stepTemplate)
		}

		// 存储这个 Job 模板和它依赖的 Step 模板
		jobTemplates[jobTemplateName] = jobTemplate
		argoWF.Spec.Templates = append(argoWF.Spec.Templates, jobTemplate)
		argoWF.Spec.Templates = append(argoWF.Spec.Templates, stepTemplates...)
	}

	// 4. 设置 Entrypoint (入口点)
	if len(jobNames) == 1 {
		// 单 Job 工作流：直接以该 Job 模板为入口
		argoWF.Spec.Entrypoint = jobNames[0]
	} else {
		// 多 Job 工作流：创建一个 DAG (有向无环图)
		dagTemplate := wfv1.Template{
			Name: "main-dag",
			DAG:  &wfv1.DAGTemplate{},
		}
		for _, jobTplName := range jobNames {
			dagTask := wfv1.DAGTask{
				Name:     jobTplName,
				Template: jobTplName,
			}
			// 添加 GHA 的 'needs' 依赖
			if deps, ok := jobDependencies[jobTplName]; ok && len(deps) > 0 {
				dagTask.Dependencies = deps
			}
			dagTemplate.DAG.Tasks = append(dagTemplate.DAG.Tasks, dagTask)
		}

		argoWF.Spec.Entrypoint = dagTemplate.Name
		argoWF.Spec.Templates = append(argoWF.Spec.Templates, dagTemplate)
	}

	// Argo v3.5+ 需要设置 Parallelism
	parallelism := int64(50) // 修复：使用 int64 而不是 IntOrString
	argoWF.Spec.Parallelism = &parallelism

	return argoWF, nil
}

// --- 辅助函数 ---

var (
	// 用于 K8s 名称清理 (DNS-1123 标签)
	nonDNSSafeRegex = regexp.MustCompile(`[^a-z0-9-]+`)
	edgeDashRegex   = regexp.MustCompile(`^-+|-+$`)
)

// sanitizeName 将 GHA 名称转换为 Argo/K8s 兼容的名称
func sanitizeName(name string) string {
	if name == "" {
		return "unnamed"
	}
	name = strings.ToLower(name)
	name = nonDNSSafeRegex.ReplaceAllString(name, "-")
	name = edgeDashRegex.ReplaceAllString(name, "")
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

// mapRunsOnToImage 简单映射 GHA 'runs-on' 到容器镜像
func mapRunsOnToImage(runsOn string) string {
	if strings.Contains(runsOn, "ubuntu-22.04") || strings.Contains(runsOn, "ubuntu-latest") {
		return "ubuntu:22.04"
	}
	if strings.Contains(runsOn, "ubuntu-20.04") {
		return "ubuntu:20.04"
	}
	// 默认值
	return "alpine:latest"
}
