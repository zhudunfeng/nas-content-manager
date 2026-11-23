/** 创建管理员或修改新密码时展示的长度与字符类别要求。 */
export const newPasswordHint: string = '密码需为 10–72 位，同时包含大写字母、小写字母、数字和英文特殊字符（如 !@#$%），不允许中文、空格或表情'

/** 校验新密码长度及四类 ASCII 字符，拒绝中文、空白和其他非允许字符。 */
export function validNewPassword(password: string): boolean {
  const bytes: number = new TextEncoder().encode(password).byteLength
  // 白名单以外的字符直接拒绝，不删除或替换用户输入。
  if (bytes < 10 || bytes > 72 || /[^\x21-\x7e]/.test(password)) return false
  const special: boolean = Array.from(password).some((character: string): boolean => {
    const code: number = character.codePointAt(0) ?? 0
    return (code >= 0x21 && code <= 0x2f) || (code >= 0x3a && code <= 0x40) || (code >= 0x5b && code <= 0x60) || (code >= 0x7b && code <= 0x7e)
  })
  return /[A-Z]/.test(password) && /[a-z]/.test(password) && /[0-9]/.test(password) && special
}
