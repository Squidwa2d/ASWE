package pm

import (
	"fmt"
	"regexp"
	"strings"
)

// buildClarifyPrompt 生成澄清追问的提示词.
// forceQuestions 为 true 时, 上层调度器还不允许进入 proposal; 注意不要把轮数门槛暴露给模型,
// 避免模型把"至少 N 轮"误解为"问满 N 轮即可结束".
func buildClarifyPrompt(qa []qaPair, forceQuestions bool) string {
	var history strings.Builder
	for _, p := range qa {
		history.WriteString(fmt.Sprintf("[%s]\n%s\n\n", strings.ToUpper(p.Role), p.Content))
	}

	var policy string
	if forceQuestions {
		policy = `本轮必须继续澄清, 不允许输出 READY.
请先基于对话历史识别仍不确定的信息缺口, 再只挑选 1~3 个最关键、最能影响实现决策的问题继续追问.
不要因为已经问过若干轮就收束; 只有信息真的足够支撑可执行 proposal 时, 后续轮次才可能 READY.

重点检查这些维度是否仍有缺口:
- 目标用户与真实使用场景
- 核心功能、边界场景与不做什么
- 数据模型 / 持久化 / 同步策略
- 权限、隐私、安全、性能、容量、离线等非功能需求
- 可验证的验收标准与成功指标
- 技术约束、已有系统集成点、上线形态

只输出如下格式, 不要寒暄:
STATUS: NEEDS_MORE_INFO
MISSING:
- 信息缺口
QUESTIONS:
- 第一个问题
- 第二个问题`
	} else {
		policy = `评估当前信息是否足以写出一份完整、可执行的 OpenSpec proposal.
不要把对话轮数当成完成依据; 只根据信息充分度判断.

必须同时满足以下条件才允许 READY:
- Why / What Changes 能写清楚, 且不是泛泛描述
- 目标用户、主要场景、核心流程明确
- 功能边界和关键边界场景明确
- 验收标准具体、可验证
- 关键非功能需求与技术/集成约束没有阻塞性未知项

如果信息已足够, 只输出:
STATUS: READY

如果还不够, 输出:
STATUS: NEEDS_MORE_INFO
MISSING:
- 信息缺口
QUESTIONS:
- 第一个问题
- 第二个问题

问题要具体, 不要问"你还希望怎样"这种空话; 不要输出任何其它内容.`
	}

	return fmt.Sprintf(`你是一名资深产品经理, 任务是把用户的模糊需求澄清成可执行的软件需求.
对话历史如下:
%s
请严格按以下规则输出, 不要有多余寒暄:

%s`, history.String(), policy)
}

// defaultFallbackQuestions 当模型在"必须追问"的阶段却没给出任何问题时, 用这组默认问题兜底,
// 保证用户真的被问够 minTurns 轮. 依据当前已完成的轮数错位出题, 避免重复.
func defaultFallbackQuestions(completed int) []string {
	banks := [][]string{
		{
			"目标用户是谁?他们在什么场景下使用这个产品, 每天/每周会用多少次?",
			"最核心的 1~2 个使用流程能不能用'用户先做 X, 再做 Y, 最后看到 Z'的方式描述一遍?",
			"有没有必须接入的已有系统或账号体系(例如公司 SSO、微信、已有数据库)?",
		},
		{
			"核心数据是什么形状?需要持久化吗?多人之间怎么同步(实时/手动刷新/离线可用)?",
			"有没有关键的边界场景需要明确处理(例如并发编辑冲突、网络断开、权限不足)?",
			"对性能/容量有具体预期吗(例如单团队上限人数、单天操作量、响应时间)?",
		},
		{
			"这个产品什么样就算'做成了'?能给出 2~3 条可验证的验收标准吗?",
			"隐私与权限上有没有红线(谁能看/改/删什么数据, 是否需要审计日志)?",
			"上线形态是什么(Web/小程序/桌面/CLI)?是否要考虑国际化或多端一致?",
		},
	}
	idx := completed
	if idx < 0 {
		idx = 0
	}
	if idx >= len(banks) {
		idx = len(banks) - 1
	}
	return banks[idx]
}

// parseClarifyResponse 解析模型输出, 返回追问列表; 若已 ready 则第二个返回值为 true.
// 空输入既不视为 ready, 也不产生问题, 让上层再次尝试或报错.
func parseClarifyResponse(s string) ([]string, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil, false
	}
	// 仅当结构化状态或一整行明确 READY 时才视为就绪.
	upper := strings.ToUpper(trimmed)
	if upper == "READY" || strings.HasPrefix(upper, "READY\n") || regexp.MustCompile(`(?mi)^\s*STATUS:\s*READY\s*$`).MatchString(trimmed) {
		return nil, true
	}
	questionBlock := extractSection(trimmed, "QUESTIONS")
	if questionBlock == "" {
		questionBlock = trimmed
	}
	// 提取每一行以 "-" 或 "*" 或 数字. 开头的条目.
	re := regexp.MustCompile(`(?m)^\s*(?:[-*]|\d+[\.)])\s+(.+)$`)
	matches := re.FindAllStringSubmatch(questionBlock, -1)
	var qs []string
	for _, m := range matches {
		q := strings.TrimSpace(m[1])
		if q != "" {
			qs = append(qs, q)
		}
	}
	// 如果没匹配到列表但有内容, 退化成把非空行都当作问题
	if len(qs) == 0 {
		for _, line := range strings.Split(questionBlock, "\n") {
			line = strings.TrimSpace(line)
			upperLine := strings.ToUpper(line)
			if line == "" || strings.HasPrefix(upperLine, "QUESTIONS") ||
				strings.HasPrefix(upperLine, "STATUS:") || strings.HasPrefix(upperLine, "MISSING:") {
				continue
			}
			qs = append(qs, line)
		}
	}
	return qs, false
}

func extractSection(s, name string) string {
	re := regexp.MustCompile(`(?mis)^\s*` + regexp.QuoteMeta(name) + `\s*:\s*\n(.*?)(?:\n\s*[A-Z_ ]+\s*:\s*\n|\z)`)
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// buildProposalPrompt 生成 proposal.md 的最终提示词.
func buildProposalPrompt(qa []qaPair) string {
	var history strings.Builder
	for _, p := range qa {
		history.WriteString(fmt.Sprintf("[%s]\n%s\n\n", strings.ToUpper(p.Role), p.Content))
	}
	return fmt.Sprintf(`基于下面的需求澄清对话, 输出一份符合 OpenSpec 规范的 proposal.md.

对话上下文:
%s
要求:
- 仅输出 markdown 内容, 首行是一个 H1 标题.
- 章节顺序: "## Why", "## What Changes", "## Target Users", "## Acceptance Criteria", "## Non-Functional Requirements", "## Impact".
- "## What Changes" 用无序列表列出拟新增的能力.
- 中文写作, 语言精炼, 切忌空话.
- 不要在 markdown 外添加解释或代码围栏.`, history.String())
}
