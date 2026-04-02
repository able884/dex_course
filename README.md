# dex_course
solana开发

生成 model crud 代码
```bash
goctl model mysql ddl --src=model/sql/sol.sql --dir=model/solmodel --home template
```

基于idl生成go客户端代码

```bash
cd pkg/pumpfun
anchor-go --idl pump.amm.idl.json --output ./generated/pumpfun_amm --program-id pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA
anchor-go --idl pump.idl.json --output ./generated/pumpfun --program-id 6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P
```

基于idl生成go-clmm客户端代码

```bash
cd pkg/raydium/clmm/idl
anchor-go --idl idl.json --output ./generated/amm_v3 --program-id CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK
```

基于idl生成go-cpmm客户端代码

```bash
cd pkg/raydium/cpmm/idl
anchor-go --idl idl.json --output ./generated/raydium_cp_swap --program-id CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C
```

根据 proto 生成 market 服务

```bash
cd market
goctl rpc protoc market.proto --go_out=.  --go-grpc_out=.  --zrpc_out=.
```

基于 proto 文件生成 pb 文件

```bash
cd market
protoc --descriptor_set_out=../gateway/internal/embed/pb/market.pb market.proto
```

根据 proto 生成 gRPC 服务

```bash
cd trade
goctl rpc protoc trade.proto --go_out=.  --go-grpc_out=.  --zrpc_out=.
```

基于 proto 文件生成 pb 文件

```bash
cd trade
protoc --descriptor_set_out=../gateway/internal/embed/pb/trade.pb trade.proto
```