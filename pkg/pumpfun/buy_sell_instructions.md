# PumpFun `buy`/`sell` 指令详解

本文按照 `rc_dex/pkg/pumpfun` 中的两个 IDL 文件（`pump.idl.json` 与 `pump.amm.idl.json`）整理了 `buy`、`sell` 指令所需的全部参数与账户，并结合生成代码中的类型定义说明它们在实际交易过程中的作用。方便在构建交易或排查问题时能够快速定位每一个字段的意义。

---

## 一、`pump.idl.json`（传统 Bonding Curve 合约）

### 1. `buy` 指令

#### 参数
1. **`amount (u64)`**  
   用户希望从当前 bonding curve 中拿到的代币数量（单位为 mint 的最小单位）。链上逻辑会基于当前的曲线状态（虚拟储备与真实储备）计算需要付出的 SOL。
2. **`max_sol_cost (u64)`**  
   用户可接受的最大 SOL（Lamports）成本，用于做滑点控制；若根据 `amount` 计算出来的成本超过该值，交易会直接失败，避免被高滑点成交。
3. **`track_volume (OptionBool)`**  
   是否同时更新 `global_volume_accumulator` 与 `user_volume_accumulator`。在 IDL 生成的类型中它只是一个包裹了 `bool` 的结构体，允许在某些场景下跳过交易量统计，以节省额外的 PDA 写入。

#### 账户说明
- **`global`（PDA，种子 `b"global"`）**  
  全局配置账户（参见 `generated/pump/types.go` 中的 `Global` 结构体），包含手续费配置、初始虚拟储备、是否允许迁移等。对 `buy` 和 `sell` 来说，它提供了程序级别的参数，且通常也是校验交易是否被全局禁用的入口。
- **`fee_recipient`（writable）**  
  接收平台手续费的普通系统账户。`buy` 操作发生时，程序会从用户付款中拆出一部分转入该账户。
- **`mint`**  
  当前 bonding curve 所对应的 SPL Token Mint。用于校验用户和曲线使用的是同一资产。
- **`bonding_curve`（PDA，writable）**  
  储存曲线状态的账户（`BondingCurve` 结构，包含虚拟/真实储备、总供应、是否完成等字段）。`buy` 操作需要更新其中的储备量与 supply。
- **`associated_bonding_curve`（writable ATA）**  
  bonding curve 自己的 SPL Token 账户，PDA 派生规则与普通 ATA 一致（`[bonding_curve, token_program, mint]`，由关联账户程序派生）。作为曲线在链上实际持有库存代币的仓位：买单时从这里把代币转给用户；卖单时代币回流到这里。
- **`associated_user`（writable ATA）**  
  用户的 SPL Token 账户，必须与 `mint` 匹配。买单会把新到手的代币打入该账户，卖单时则从该账户扣除。
- **`user`（signer + writable）**  
  钱包签名者，既是 SOL 付款人也是代币持有者。其余账户多数通过 `user` 推导（比如 `user_volume_accumulator`）。
- **`system_program`（常量地址 `1111…`）**  
  用于 SOL 转账、创建账户等系统级 CPI。
- **`token_program`（常量地址 `Tokenkeg...`）**  
  SPL Token Program（Token-2022 同样通过 IDL 声明），负责代币转账与铸造等 CPI。
- **`creator_vault`（writable PDA）**  
  以 `[b"creator-vault", bonding_curve.creator]` 为种子派生的存储账户，持有属于项目方的库存或手续费份额。交易时可能需要在这里累加 creator fee。
- **`event_authority`（PDA，种子 `b"__event_authority"`）**  
  Anchor 事件授权 PDA，Anchor 在 CPI 内部 emit 事件时会校验它以防伪造。
- **`program`（常量地址 `6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P`）**  
  PumpFun 主程序 ID，本合约 CPI 到真实的 on-chain Pump 合约。
- **`global_volume_accumulator`（writable，PDA 种子 `b"global_volume_accumulator"`）**  
  全局交易量统计账户（`GlobalVolumeAccumulator` 结构，内部保存 30 天滚动窗口的供应和 SOL 成交量）。仅在 `track_volume` 为真时会被写入。
- **`user_volume_accumulator`（writable，PDA 种子 `b"user_volume_accumulator", user`）**  
  用户级交易量记录（`UserVolumeAccumulator` 结构，含累计/可领取的激励额度、最近更新时间等）。用于发放 volume incentive。
- **`fee_config`（PDA，种子 `["fee_config", <32 字节常量>]`，由 `fee_program` 派生）**  
  由外部 `pfee...` 程序维护的动态费率配置。`buy` 指令会把它作为 CPI 参数传给 fee program，决定手续费和多钱包分配。
- **`fee_program`（常量地址 `pfeeUxB6...`）**  
  PumpFun 专用的 fee 配置程序。主程序在执行交易时会调用它来读取或校验 `fee_config`，以保证费率参数未被篡改。

### 2. `sell` 指令

#### 参数
1. **`amount (u64)`** — 用户想要出售的代币数量。  
2. **`min_sol_output (u64)`** — 期望至少拿回的 SOL（Lamports），用来限制滑点；若实际回收金额低于该值则交易失败。

#### 账户
`sell` 的账户集合和 `buy` 重叠度很高，但因为卖出时不会更新 volume，也不需要 `associated_user` 以外的 ATA：

- **`global`**、**`fee_recipient`**、**`mint`**、**`bonding_curve`**、**`associated_bonding_curve`**、**`associated_user`**、**`user`**、**`system_program`**、**`creator_vault`**、**`token_program`**、**`event_authority`**、**`program`**、**`fee_config`**、**`fee_program`**  

它们的作用与 `buy` 中相同，区别在于：
- 卖出时 `user` 需要先在 `associated_user` 授权足够的代币供 CPI 扣除；
- `bonding_curve` 的虚拟/真实储备、`creator_vault` 中的余额会因为退还 SOL 而发生相反方向的更新；
- `global_volume_accumulator` / `user_volume_accumulator` 不在账户列表，意味着 `sell` 交易不会写入 volume 统计。

---

## 二、`pump.amm.idl.json`（AMM 合约）

该 IDL 用于 Pump 的 AMM 版本，在池子完成 bonding curve 后迁移到 AMM。`buy`/`sell` 的语义对应从 AMM 池买入/卖出 base token，对应账户明显围绕 `pool` 状态与 LP 资金仓位展开。

### 1. `buy` 指令

#### 参数
1. **`base_amount_out (u64)`**  
   期望买到的 base token 数量。合约会根据池内储备和当前曲线用恒定乘积公式或内置曲线计算需要支付的 quote token。
2. **`max_quote_amount_in (u64)`**  
   单笔最多愿意支付的 quote token（通常是 SOL 或 wSOL），超出即交易失败以保障滑点。
3. **`track_volume (OptionBool)`**  
   与 bonding curve 版本相同；设置为 true 时会要求附带 volume 相关 PDA 并在成功后写入。

#### 账户说明
- **`pool`**  
  AMM 池状态账户（`Pool` 结构），包含池子 bump、创建者、Base/Quote mint、LP mint 以及池内账户地址、LP 供应、coin creator 等字段。每一次交易都会读取/修改它的储备指针（比如 `PoolBaseTokenAccount`）。
- **`user`（signer + writable）**  
  买方钱包，提供 quote token 并接收 base token。
- **`global_config`**  
  AMM 全局配置（`GlobalConfig` 结构），提供协议级别的费率（LP/Protocol/Coin creator）、禁用标志位和 fee recipient 列表。`buy` 时用于校验是否允许交易以及计算 fee。
- **`base_mint` / `quote_mint`（`relations: ['pool']`）**  
  指明池子所交易的两个资产，Anchor 会用 `relations` 保证它们与 `pool` 记录一致，避免用户传入错误 mint。
- **`user_base_token_account` / `user_quote_token_account`（writable）**  
  用户的 SPL Token 账户。`buy` 时 `user_quote_token_account` 被扣除 quote token，`user_base_token_account` 收到 base token。
- **`pool_base_token_account` / `pool_quote_token_account`（writable，`relations: ['pool']`）**  
  池子的两种资产储备账户，记录在 `Pool` 结构中。交易会在这两个账户间移动资产以维持恒定乘积。
- **`protocol_fee_recipient`**  
  平台 fee 收款地址（从 `global_config.ProtocolFeeRecipients` 中选取），只读用于校验。
- **`protocol_fee_recipient_token_account`（writable ATA）**  
  协议 fee 的实际存放 ATA，由 `[protocol_fee_recipient, quote_token_program, quote_mint]` + Associated Token Program 派生，`buy` 过程中需要把协议费那部分 quote token 打入此处。
- **`base_token_program` / `quote_token_program`**  
  Base 与 Quote 所使用的 SPL Token Program（可分别是 Token-2022）。这些 program id 也被用到派生 ATA。
- **`system_program`** 与 **`associated_token_program`（常量地址 `ATokenGPv...`）**  
  分别用于创建账户/转 lamports 与创建 ATA。
- **`event_authority`** 与 **`program`（常量地址 `pAMMBay6...`）**  
  Anchor CPI 所需的事件授权 PDA 与 AMM 程序 ID。
- **`coin_creator_vault_authority`（PDA，种子 `[b"creator_vault", pool.coin_creator]`）**  
  代表 coin creator 的受控地址，用于存放迁移到 AMM 后仍需锁仓或结算的份额。
- **`coin_creator_vault_ata`（writable ATA）**  
  由 `coin_creator_vault_authority` 派生的 quote token ATA，`buy` 过程里若需要向 coin creator 分润则会把对应的 quote token 发到此处。
- **`global_volume_accumulator`（writable）** 与 **`user_volume_accumulator`（writable）**  
  与 bonding curve 版本含义一致，用于统计 AMM 阶段的成交量，方便计算激励或排行榜。只在 `track_volume` 为真时需要在账户列表里提供。
- **`fee_config`（PDA，种子 `["fee_config", <32 字节常量>]`，由 `fee_program` 派生）**  
  AMM 使用的费率配置，常量种子与 bonding curve 版本不同（可在 IDL 看到第二个种子字节数组不同），以区分不同产品线的 fee 表。
- **`fee_program`（常量地址 `pfeeUxB6...`）**  
  同一 fee 程序，负责校验 `fee_config` 与返回费率计算结果。

### 2. `sell` 指令

#### 参数
1. **`base_amount_in (u64)`** — 用户要卖出的 base token 数量。  
2. **`min_quote_amount_out (u64)`** — 期望至少得到的 quote token 数量，用于滑点保护。

#### 账户
`sell` 使用的账户几乎和 `buy` 相同，只是：
- **不再需要 `track_volume` 参数，也没有 volume 相关 PDA** —— AMM 版本的 `sell` 没有强制更新交易量。
- 资产流向反转：从 `user_base_token_account` 扣 token，存入 `pool_base_token_account`；quote token 从池子与协议 fee 账户流向用户。

其余账户的作用与 `buy` 描述完全一致：
- `pool` / `global_config` / `base_mint` / `quote_mint` / `user` / `user_*` / `pool_*`
- `protocol_fee_recipient` + `protocol_fee_recipient_token_account`
- `base_token_program` / `quote_token_program` / `system_program` / `associated_token_program`
- `event_authority` / `program`
- `coin_creator_vault_authority` / `coin_creator_vault_ata`
- `fee_config` / `fee_program`

---

## 三、使用建议
- 在构建交易时，可按照上文顺序依次准备账户，更容易与 IDL 对应；尤其注意 ATA（如 `associated_bonding_curve`、`protocol_fee_recipient_token_account` 等）都具有固定的派生方式，提前根据 seeds 推导可减少 RPC round-trip。
- `track_volume` 为真时必须同时携带 volume 相关 PDA，否则会因账户缺失报错；若不想记录交易量，可将其置为 `false` 并省略对应账户，用于节省 2 个写账户额度。
- `fee_config` 与 `fee_program` 在两个 IDL 中虽然名字一样，但第二个种子常量不同，意味着不能混用。确保针对不同产品线取对的 PDA，否则 `pfee` 程序校验会失败。
- `max_sol_cost` / `min_sol_output` / `max_quote_amount_in` / `min_quote_amount_out` 等滑点参数是保护交易者的最后防线，通常应当由前端根据预估报价 + 允许滑点计算得出，切勿直接使用 0。

以上内容覆盖了 `rc_dex/pkg/pumpfun` 中两个 IDL 的 `buy`/`sell` 指令在参数与账户层面的全部作用，可用于生成交易、理解跨程序调用或审计账户依赖。

