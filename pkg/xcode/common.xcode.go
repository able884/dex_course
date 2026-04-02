package xcode

// 请求成功 10000
var (
	OK = add(Ok, "成功", "OK")
)

// system error 0-999
var (
	NoLogin                    = add(101, "用户未登录", "NOT_LOGIN")
	NoContent                  = add(204, "无内容", "No Content")
	RequestErr                 = add(400, "无效参数", "Bad Request")
	InvalidSignatureError      = add(401, "无效的签名", "Invalid Signature")
	AccessDenied               = add(403, "权限被拒绝", "PERMISSION_DENIED")
	NotFound                   = add(404, "未找到", "NOT_FOUND")
	MethodNotAllowed           = add(405, "方法不允许", "METHOD_NOT_ALLOWED")
	TooManyRequests            = add(429, "用户请求过多", "Too Many User Request")
	Canceled                   = add(498, "已取消", "CANCELED")
	ServerErr                  = add(500, "内部错误", "Internal Error")
	ServiceUnavailable         = add(503, "不可用", "UNAVAILABLE")
	Deadline                   = add(504, "加载时间过长，请重试", "The loading time is too long, please try again")
	LimitExceed                = add(509, "资源耗尽", "RESOURCE_EXHAUSTED")
	DbError                    = add(512, "请稍后重试", "An error occurred, please try again") //数据库操作异常
	RedisError                 = add(513, "请稍后重试", "An error occurred, please try again") //redis操作异常
	RPCError                   = add(514, "请稍后重试", "An error occurred, please try again") //rpc操作异常
	InternalError              = add(515, "请稍后重试", "An error occurred, please try again") //内部错误
	RequestError               = add(516, "请稍后重试", "An error occurred, please try again") //请求错误
	RegionalAccessRestrictions = add(519, "很遗憾，我们无法向受限制国家/地区(包括使用受限国家IP)的用户提供服务", "Sorry, we are unable to provide services to users in restricted countries/regions (including those using IPs from restricted countries)")
	NotingFoundError           = add(520, "请稍后重试", "An error occurred, please try again") //没有数据
)

// account 1000-1999
var (
	EmailVerificationInvalid         = add(1001, "邮箱验证不合法", "Invalid email address")
	EmailSendError                   = add(1002, "邮件发送失败", "Email sending failed")
	EmailDispatchExcessive           = add(1003, "邮件发送太频繁，请稍后再试", "Emails are being sent too frequently, please try again later")
	EmailCodeExpired                 = add(1004, "验证码失效", "Verification code expired")
	EmailCodeError                   = add(1005, "验证码错误", "Incorrect verification code")
	EmailAlreadyRegisteredError      = add(1006, "邮箱已注册", "The email address already registered")
	EmailIncorrectPasswordError      = add(1007, "账号或密码错误", "Email address or password is incorrect, please input again")
	EmailNotFoundError               = add(1008, "邮箱地址不存在", "Email address doesn’t exist")
	TransferInsufficientBalanceError = add(1009, "代币余额不足", "Insufficient token balance")
	TransferToYourselfError          = add(1010, "不能给自己转账", "You can’t transfer to yourself")
	TransferMinWithdrawAmountError   = add(1011, "提现金额低于最低限额", "Withdrawal amount is below the minimun limit")
	InvitationCodeError              = add(1012, "邀请码错误", "Invitation code error")
	EmailRateLimitError              = add(1013, "请稍后重试", "An error occurred, please try again")
	EmailInvalidSignatureError       = add(1046, "你已绑定 google验证，请输入谷歌验证码后登录", "You have linked Google Authenticator. Please enter the Google verification code to log in")
	EmailGoogleBindError             = add(1047, "请先绑定 google验证器后尝试", "Please set up the Google Authenticator first, then try again")
	GoogleCodeVerifyError            = add(1048, "谷歌验证码错误", "Wrong google authenticator code")
)

// market 2000-2999
var (
	ChainIdNotSupport = add(2000, "暂不支持此链", "This network is currently not supported")
	GetKlineDataError = add(2001, "获取kline数据失败", "Kline data is null")
)

// websocket 3000-3999

// dcmsg+twmsg+smartmoney 4000-4999
var (
	DuplicatedMessage      = add(4000, "重复的消息", "Duplicate Message")
	FollowUserIdEmpty      = add(4001, "关注用户id为空", "User ID for Following is Empty")
	FollowedUserIdEmpty    = add(4002, "被关注用户id为空", "Followed User ID is Empty")
	CannotFollowSelf       = add(4003, "不能关注自己", "Cannot Follow Oneself")
	UserIdEmpty            = add(4004, "用户id为空", "User ID is Empty")
	SmartMoneyAddressEmpty = add(4004, "聪明钱地址为空", "Smart Money Address is Empty")
)

// trade 6000-6999
var (
	BalanceNotEnough            = add(6000, "代币余额不足", "Insufficient token balance")
	OrderTypeNotSupportCancel   = add(6001, "该订单类型不支持撤单", "This order type does not support cancellation")
	OrderStatusNotSupportCancel = add(6002, "该订单状态不支持撤单", "This order status does not support cancellation")
	TooLittle                   = add(6003, "下单数量太少,请增加后重试", "The order quantity is too small,pls add and try again")
	AntiErr                     = add(6004, "防夹功能异常，请关闭此功重新尝试", "The Anti-Mev function is abnormal, pls disable it and try again")
	TokenAccountFrozen          = add(6005, "该代币已被冻结，请保持谨慎", "This token has been frozen, pls proceed with caution")
	TransferAddressErr          = add(6006, "转账地址错误,请检查输入的地址是否正确", "Address error, pls check it")
	AmountErr                   = add(6007, "数量有问题,请检查输入的数量是否正确", "Insufficient amount,pls check it")
	TokenAddressErr             = add(6008, "币种地址错误,请检查币种是否正确", "Token address error, pls check it")
	SlippageLimit               = add(6009, "滑点限制，请调大或者稍后再试", "Slippage limit, pls increase or wait then try again")
	PumpPoolZeroErr             = add(6010, "pump池子进度已满，请等待发射后重试", "Progress is complete, wait for launch and try again")
	PoolNotFound                = add(6011, "池子不存在", "pool not found")
	SolBalanceNotEnough         = add(6012, "SOL 余额不足", "Insufficient SOL balance")
	SolGasNotEnough             = add(6013, "SOL 余额不足支付 Gas 费用", "Insufficient SOL for gas fees")
	TxTimeout                   = add(6014, "手续费太低，交易失败，请调整后重试", "Transaction failed due to low gas fees. Please adjust and try again.")
	BNBBalanceNotEnough         = add(6015, "BNB 余额不足", "Insufficient BNB balance")
	BNBGasNotEnough             = add(6016, "BNB 余额不足支付 Gas 费用", "Insufficient BNB for gas fees")
	ETHBalanceNotEnough         = add(6017, "ETH 余额不足", "Insufficient ETH balance")
	ETHGasNotEnough             = add(6018, "ETH 余额不足支付 Gas 费用", "Insufficient ETH for gas fees")
	BaseEthBalanceNotEnough     = add(6019, "ETH(Base) 余额不足", "Insufficient ETH(Base) balance")
	BaseEthGasNotEnough         = add(6020, "ETH(Base) 余额不足支付 Gas 费用", "Insufficient ETH(Base) for gas fees")
	BaseEthTransferGasNotEnough = add(6021, "ETH(Base) 由于L1层额外手续费原因无法全转出,请减少一点再试", "You can't transfer ETH(Base) totally due to additional L1 fees, pls reduce and try again")
	PoolLiquidityNotEnough      = add(6022, "池子流动性不足", "Pool liquidity not enough")
)

// rebate 7000-7999
var (
	RebateWithdrawAmountError                                = add(7001, "提现金额错误", "The withdrawal amount is incorrect")
	RebateWithdrawRecipientAddressError                      = add(7002, "提现接收地址错误", "The withdrawal receiving address is incorrect")
	RebateWithdrawRecipientAddressNotMatchWalletAddressError = add(7003, "提现接收地址不匹配钱包地址", "The withdrawal receiving address does not match the wallet address")
	RebateWithdrawAmountLessThanMinError                     = add(7005, "提现金额小于最小金额", "The withdrawal amount is below the minimum limit")
	RebateWithdrawAmountGraterThanMaxError                   = add(7006, "提现金额大于最大金额", "The withdrawal amount exceeds the maximum limit")
	RebateWithdrawAmountGreaterThanAvailableError            = add(7007, "提现金额大于可用余额", "The withdrawal amount exceeds the available balance")
	RebateWithdrawSendTxError                                = add(7008, "提现发送交易失败", "The withdrawal transaction is failed, please try again")
)

// adminpannel 仅用于后台管理，非对外部用户
var (
	AdminPermissionDenied = add(8001, "用户权限不足", "User permission denied")
)

// sign 9000-9999
var (
	ExportReachLimit   = add(9000, "平台导出私钥次数达到限制，请明天再试", "export private key has reached limit today , pls try again tomorrow")
	TreasuryReachLimit = add(9001, "平台返佣提现次数达到限制，请明天再试", "rebate withdraw has reached limit today , pls try again tomorrow")
)
