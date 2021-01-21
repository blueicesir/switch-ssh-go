package ssh

import (
	"fmt"
	"strings"
	"time"
)

const (
	HUAWEI = "huawei"
	H3C    = "h3c"
	CISCO  = "cisco"
)

// 如果需要禁止调试信息输出ssh.IsLogDebug=false
var IsLogDebug = true

/**
 * 外部调用的统一方法，完成获取会话（若不存在，则会创建连接和会话，并存放入缓存），执行指令的流程，返回执行结果
 * @param user ssh连接的用户名, password 密码, ipPort 交换机的ip和端口, cmds 执行的指令(可以多个)
 * @return 执行的输出结果和执行错误
 * @author shenbowei
 */
func RunCommands(user, password, ipPort string, cmds ...string) (string, error) {
	sessionKey := user + "_" + password + "_" + ipPort
	// 设置会话参数，锁定会话
	sessionManager.LockSession(sessionKey)
	// 退出此函数时自动解锁会话
	defer sessionManager.UnlockSession(sessionKey)

	// 先检查是否存在会话信息
	sshSession, err := sessionManager.GetSession(user, password, ipPort, "")
	if err != nil {
		LogError("GetSession error:%s", err)
		return "", err
	}
	// 往会话中写入需要执行的命令
	sshSession.WriteChannel(cmds...)
	// 最多等待2秒获取返回结果
	result := sshSession.ReadChannelTiming(2 * time.Second)
	filteredResult := filterResult(result, cmds[0])
	return filteredResult, nil
}

/**
 * 外部调用的统一方法，完成获取会话（若不存在，则会创建连接和会话，并存放入缓存），执行指令的流程，返回执行结果
 * @param user ssh连接的用户名, password 密码, ipPort 交换机的ip和端口, brand 交换机品牌（可为空）， cmds 执行的指令(可以多个)
 * @return 执行的输出结果和执行错误
 * @author shenbowei
 */
func RunCommandsWithBrand(user, password, ipPort, brand string, cmds ...string) (string, error) {
	sessionKey := user + "_" + password + "_" + ipPort
	sessionManager.LockSession(sessionKey)
	defer sessionManager.UnlockSession(sessionKey)

	sshSession, err := sessionManager.GetSession(user, password, ipPort, brand)
	if err != nil {
		LogError("GetSession error:%s", err)
		return "", err
	}
	sshSession.WriteChannel(cmds...)
	result := sshSession.ReadChannelTiming(2 * time.Second)
	filteredResult := filterResult(result, cmds[0])
	return filteredResult, nil
}

/**
 * 外部调用的统一方法，完成获取交换机的型号
 * @param user ssh连接的用户名, password 密码, ipPort 交换机的ip和端口
 * @return 设备品牌（huawei，h3c，cisco，""）和执行错误
 * @author shenbowei
 */
func GetSSHBrand(user, password, ipPort string) (string, error) {
	sessionKey := user + "_" + password + "_" + ipPort
	sessionManager.LockSession(sessionKey)
	defer sessionManager.UnlockSession(sessionKey)

	sshSession, err := sessionManager.GetSession(user, password, ipPort, "")
	if err != nil {
		LogError("GetSession error:%s", err)
		return "", err
	}
	return sshSession.GetSSHBrand(), nil
}

/**
 * 对交换机执行的结果进行过滤
 * @paramn result:返回的执行结果（可能包含脏数据）, firstCmd:执行的第一条指令
 * @return 过滤后的执行结果
 * @author shenbowei
 */
func filterResult(result, firstCmd string) string {
	//对结果进行处理，截取出指令后的部分
	filteredResult := ""
	// 把捕获的返回内容根据\n分解成多个字符串
	resultArray := strings.Split(result, "\n")
	findCmd := false
	promptStr := ""
	for _, resultItem := range resultArray {
		// 替换每行文本中的\b就是Basckspace退格为空，替换所有。
		resultItem = strings.Replace(resultItem, " \b", "", -1)

		// 过滤Terminal Color控制符,这个不是通用函数，仅仅用于华为USG6360设备的disp cur | include 指令。
		// \0x1b[1D 控制表示一个Backspace正好是删除前面一个空格字符。后续可修改为检测是否存在0x1b[1D字符，如果存在，查找并删除前一个字符做到相对通用，抑或直接应用其它第三方Terminal CSI控制库进行过滤输出
		if strings.Contains(resultItem," [1D"){
			resultItem = strings.Replace(resultItem, " [1D", "", -1)
		}
		
		// 这里应该是替换提示符，但似乎原作者实现有些问题，实际上没有作用
		if findCmd && (promptStr == "" || strings.Replace(resultItem, promptStr, "", -1) != "") {
			filteredResult += resultItem + "\n"
			continue
		}
		// 如果命令中包含传入指令的第一行数据，则替换
		if strings.Contains(resultItem, firstCmd) {
			findCmd = true
			promptStr = resultItem[0:strings.Index(resultItem, firstCmd)]
			promptStr = strings.Replace(promptStr, "\r", "", -1)
			promptStr = strings.TrimSpace(promptStr)
			LogDebug("Find promptStr='%s'", promptStr)
			//将命令添加到结果中
			filteredResult += resultItem + "\n"
		}
		

		
	}
	if !findCmd {
		return result
	}
	return filteredResult
}

func LogDebug(format string, a ...interface{}) {
	if IsLogDebug {
		fmt.Println("[DEBUG]:" + fmt.Sprintf(format, a...))
	}
}

func LogError(format string, a ...interface{}) {
	fmt.Println("[ERROR]:" + fmt.Sprintf(format, a...))
}
