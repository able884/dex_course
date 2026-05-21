docker build -t anchor-go-app .

docker run --rm -v ./idl/:/app anchor-go-app anchor-go --src /app/idl.json  # 执行命令，使用映射的 idl.json\        go get github.com/gagliardetto/solana-go@v1.4.0
go get github.com/gagliardetto/binary@v0.6.1
go get github.com/gagliardetto/treeout@v0.1.4
go get github.com/gagliardetto/gofuzz@v1.2.2
go get github.com/stretchr/testify@v1.6.1
go get github.com/davecgh/go-spew@v1.1.1