//go:build windows

package utils

import (
	"golang.org/x/sys/windows"
)

// PipeSDDL 生成只允许本地 SYSTEM、管理员组与当前用户访问的 SDDL。
//
// 命名管道默认的 DACL 允许任意本地进程读写，攻击者可借此调用
// installPlugin / togglePlugin / startCore 等高危方法。这里把管道访问
// 限制到三类主体，低权限本地进程（服务、受限令牌）将无法连接。
func PipeSDDL() (string, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	defer token.Close()

	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	sid := user.User.Sid

	// D:P 表示只允许显式 ACE；GA 为完全访问。
	return "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;" + sid.String() + ")", nil
}
