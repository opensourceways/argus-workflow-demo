package main
 
import (
	"fmt"
	"log"
	"os"
	"strings"
 
	"github.com/nektos/act/pkg/model"
)
 
func main() {
	// 方式1: 从文件解析单个工作流
	parseFromFile()
	
	// 方式2: 从字符串解析工作流
	parseFromString()
	
	// 方式3: 从目录解析所有工作流并创建执行计划
	parseFromDirectory()
}
 
// 从 YAML 字符串解析工作流
func parseFromString() {
	workflowYAML := `
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Setup Go
        uses: actions/setup-go@v3
        with:
          go-version: '1.19'
      - name: Run tests
        run: go test ./...
  
  build:
    needs: test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Build
        run: go build -o app ./cmd
`
 
	reader := strings.NewReader(workflowYAML)
	
	// 使用 act 的解析器
	planner, err := model.NewSingleWorkflowPlanner("ci.yml", reader)
	if err != nil {
		log.Fatalf("解析工作流失败: %v", err)
	}
 
	// 获取所有事件
	events := planner.GetEvents()
	fmt.Printf("支持的事件: %v\n", events)
 
	// 为 push 事件创建执行计划
	plan, err := planner.PlanEvent("push")
	if err != nil {
		log.Fatalf("创建执行计划失败: %v", err)
	}
 
	// 打印执行计划
	printPlan(plan)
}
 
// 从文件解析工作流
func parseFromFile() {
	file, err := os.Open(".github/workflows/ci.yml")
	if err != nil {
		log.Printf("无法打开文件 (跳过): %v", err)
		return
	}
	defer file.Close()
 
	planner, err := model.NewSingleWorkflowPlanner("ci.yml", file)
	if err != nil {
		log.Fatalf("解析工作流文件失败: %v", err)
	}
 
	// 获取完整执行计划
	plan, err := planner.PlanAll()
	if err != nil {
		log.Fatalf("创建完整计划失败: %v", err)
	}
 
	printPlan(plan)
}
 
// 从目录解析所有工作流
func parseFromDirectory() {
	workflowDir := ".github/workflows"
	
	// 检查目录是否存在
	if _, err := os.Stat(workflowDir); os.IsNotExist(err) {
		log.Printf("工作流目录不存在 (跳过): %s", workflowDir)
		return
	}
 
	// 创建工作流规划器 (noWorkflowRecurse=false, strict=false)
	planner, err := model.NewWorkflowPlanner(workflowDir, false, false)
	if err != nil {
		log.Fatalf("加载工作流目录失败: %v", err)
	}
 
	// 获取所有支持的事件
	events := planner.GetEvents()
	fmt.Printf("发现的事件: %v\n", events)
 
	// 为每个事件创建计划
	for _, event := range events {
		fmt.Printf("\n=== 事件: %s ===\n", event)
		plan, err := planner.PlanEvent(event)
		if err != nil {
			log.Printf("为事件 %s 创建计划失败: %v", event, err)
			continue
		}
		printPlan(plan)
	}
}
 
// 打印执行计划详情
func printPlan(plan *model.Plan) {
	fmt.Printf("执行计划包含 %d 个阶段:\n", len(plan.Stages))
	
	for i, stage := range plan.Stages {
		fmt.Printf("  阶段 %d: %d 个并行任务\n", i+1, len(stage.Runs))
		
		for _, run := range stage.Runs {
			job := run.Job()
			fmt.Printf("    - 任务: %s\n", run.String())
			fmt.Printf("      运行环境: %v\n", job.RunsOn())
			fmt.Printf("      步骤数量: %d\n", len(job.Steps))
			
			// 打印依赖关系
			if needs := job.Needs(); len(needs) > 0 {
				fmt.Printf("      依赖: %v\n", needs)
			}
			
			// 打印环境变量
			if env := job.Environment(); len(env) > 0 {
				fmt.Printf("      环境变量: %d 个\n", len(env))
			}
			
			// 打印 matrix 策略
			if matrix, err := job.GetMatrixes(); err == nil && len(matrix) > 1 {
				fmt.Printf("      Matrix 策略: %d 个组合\n", len(matrix))
			}
		}
	}
	fmt.Println()
}
 
// 高级用法: 直接操作 Workflow 对象
func advancedWorkflowAnalysis() {
	workflowYAML := `
name: Matrix Build
on: push
jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macos-latest]
        go-version: ['1.18', '1.19', '1.20']
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v3
      - name: Setup Go ${{ matrix.go-version }}
        uses: actions/setup-go@v3
        with:
          go-version: ${{ matrix.go-version }}
`
 
	reader := strings.NewReader(workflowYAML)
	
	// 直接读取 Workflow 对象
	workflow, err := model.ReadWorkflow(reader, false)
	if err != nil {
		log.Fatalf("读取工作流失败: %v", err)
	}
 
	fmt.Printf("工作流名称: %s\n", workflow.Name)
	fmt.Printf("触发事件: %v\n", workflow.On())
	
	// 分析每个 Job
	for jobID, job := range workflow.Jobs {
		fmt.Printf("\nJob: %s\n", jobID)
		
		// 获取 matrix 组合
		matrixes, err := job.GetMatrixes()
		if err != nil {
			log.Printf("获取 matrix 失败: %v", err)
			continue
		}
		
		fmt.Printf("Matrix 组合数: %d\n", len(matrixes))
		for i, matrix := range matrixes {
			fmt.Printf("  组合 %d: %v\n", i+1, matrix)
		}
		
		// 分析步骤
		for i, step := range job.Steps {
			fmt.Printf("  步骤 %d: %s\n", i+1, step.String())
			if step.Uses != "" {
				fmt.Printf("    使用: %s\n", step.Uses)
			}
			if step.Run != "" {
				fmt.Printf("    运行: %s\n", step.Run)
			}
		}
	}
}
