github workflow 与 argo workflow 转换规则
========================================

## 转换规则

### 最简单的转换规则

1. github workflow 的 on 触发时机不用管，由 controller 负责触发，可以理解都是手动触发
2. jobs 转换为 argo workflow 的 template
3. job 的 needs 字段转换为 argo workflow 的 dependencies 字段(需要结合DAG字段使用)
4. job 的if字段转换为 argo workflow 的 when 字段
5. job 的 run-on 字段转换为 k8s 的同名 configmap，在字段转换完后做 json 的merge
6. job 的 container 的image字段转换为 argo workflow 的 script 的 image 字段
7. job 的 steps 转换为 argo workflow 的 script 字段
8. job 的 steps 需要merge成一个 bash 脚本插入到 argo workflow 的 script 字段中

