# dex_course
solana开发

生成 model crud 代码
```bash
goctl model mysql ddl --src=model/sql/sol.sql --dir=model/solmodel --home 
```

基于idl生成go客户端代码

```bash
cd pkg/pumpfun
anchor-go --idl pump.amm.idl.json --output ./generated/pumpfun_amm --program-id pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA
anchor-go --idl pump.idl.json --output ./generated/pumpfun --program-id 6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P
```