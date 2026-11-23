package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// accountResetUsage 是不插入任何用户参数的固定维护命令帮助。
const accountResetUsage = "用法：nas account reset-password --data-dir <已有数据目录> [--password-stdin]"

// accountResetOptions 仅保存允许公开的维护选项，不接受命令行或环境密码。
type accountResetOptions struct {
	dataDir       string // 已有数据库所在目录，允许规范化后的本地目录别名。
	passwordStdin bool   // 显式从重定向标准输入读取一个完整密码。
	help          bool   // 只显示固定帮助，不打开任何应用资源。
}

// parseAccountResetOptions 严格解析唯一命令和选项，错误绝不回显未知值。
func parseAccountResetOptions(args []string) (accountResetOptions, error) {
	var options accountResetOptions
	if len(args) < 2 || args[0] != "account" || args[1] != "reset-password" {
		return options, errors.New("维护命令无效")
	}
	if len(args) == 3 && args[2] == "--help" {
		options.help = true
		return options, nil
	}
	seenDir, seenStdin := false, false
	for index := 2; index < len(args); index++ {
		switch args[index] {
		case "--data-dir":
			if seenDir || index+1 >= len(args) || args[index+1] == "" || strings.HasPrefix(args[index+1], "--") {
				return options, errors.New("维护参数无效")
			}
			seenDir = true
			index++
			options.dataDir = args[index]
		case "--password-stdin":
			if seenStdin {
				return options, errors.New("维护参数重复")
			}
			seenStdin, options.passwordStdin = true, true
		default:
			return options, errors.New("维护参数无效")
		}
	}
	if !seenDir {
		return options, errors.New("必须指定已有数据目录")
	}
	return options, nil
}

// RunAccountMaintenance 执行独立维护进程，输出仅含固定中文原因和安全结果。
func RunAccountMaintenance(ctx context.Context, args []string, input *os.File, output io.Writer, diagnostic io.Writer) int {
	options, err := parseAccountResetOptions(args)
	if err != nil {
		_, _ = fmt.Fprintln(diagnostic, "维护命令或参数无效。\n"+accountResetUsage)
		return 2
	}
	if options.help {
		if _, err = fmt.Fprintln(output, accountResetUsage); err != nil {
			return 1
		}
		return 0
	}
	committed, err := resetAccountOffline(ctx, options, input, diagnostic)
	logger := slog.New(slog.NewJSONHandler(diagnostic, nil))
	if err != nil {
		if committed {
			logger.Error("管理员密码已重置，但维护清理失败，请核对后重新登录", "user_id", 1)
			return 1
		}
		if errors.Is(err, errOfflineStateUncertain) {
			logger.Error("离线重置事务结果未确认，请核对账号后再执行，不能自动重试")
			return 1
		}
		// 清理失败可能和取消一起返回；只有纯取消才能声明安全取消成功。
		if err == context.Canceled || err == context.DeadlineExceeded {
			logger.Info("离线重置已取消，账号未修改")
			return 130
		}
		// 仅使用本维护路径构造的固定错误，不拼接 SQLite、输入或参数原文。
		logger.Error("离线重置失败，账号未修改", "reason", err.Error())
		return 1
	}
	logger.Info("管理员离线密码重置完成，全部旧会话已撤销", "user_id", 1)
	if _, err = fmt.Fprintln(output, "管理员密码已重置，全部旧会话已撤销，请重新登录。"); err != nil {
		return 1
	}
	return 0
}

// resetAccountOffline 依次持有规范目录租约和受限数据库，先关库再释放租约。
func resetAccountOffline(ctx context.Context, options accountResetOptions, input *os.File, diagnostic io.Writer) (committed bool, resultErr error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	root, err := canonicalDataDirectory(options.dataDir)
	if err != nil {
		return false, errors.New("已有数据目录不可访问")
	}
	if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
		return false, errors.New("数据目录必须已经存在")
	}
	path := filepath.Join(root, "library.db")
	original, err := offlineDatabaseFile(path)
	if err != nil {
		return false, err
	}
	// 复用在线实例的同一平台锁，但不调用会创建目录的 acquireInstance。
	lease, err := acquireInstanceFile(filepath.Join(root, ".nas-instance.lock"))
	if err != nil {
		return false, errors.New("数据目录被占用或独占锁不可写，请正常停服并检查权限")
	}
	defer func() {
		if closeErr := lease.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, errors.New("数据目录独占锁关闭失败"))
		}
	}()
	actual, err := offlineDatabaseFile(path)
	if err != nil || !os.SameFile(original, actual) {
		return false, errors.New("数据库文件身份已变化")
	}
	db, expected, err := openOfflineAccountDatabase(ctx, path)
	if err != nil {
		return false, err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, errors.New("维护数据库关闭失败"))
		}
	}()
	password, err := readOfflinePassword(ctx, input, diagnostic, options.passwordStdin)
	if err != nil {
		return false, err
	}
	defer clear(password)
	if !validNewPassword(string(password)) {
		return false, errors.New(newPasswordHint)
	}
	if err = ctx.Err(); err != nil {
		return false, err
	}
	// 终端已恢复；bcrypt 不持数据库写事务。
	hash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return false, errors.New("新密码摘要生成失败")
	}
	defer clear(hash)
	actual, err = offlineDatabaseFile(path)
	if err != nil || !os.SameFile(original, actual) {
		return false, errors.New("数据库文件身份已变化")
	}
	if err = resetOfflineAccountPassword(ctx, db, expected, string(hash)); err != nil {
		if errors.Is(err, errOfflineStateUncertain) {
			return false, errOfflineStateUncertain
		}
		if err == context.Canceled || err == context.DeadlineExceeded {
			return false, err
		}
		return false, errors.New("维护写入未完成，请检查数据库版本、权限或并发修改后核对账号")
	}
	return true, nil
}
