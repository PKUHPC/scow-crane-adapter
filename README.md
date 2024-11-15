# scow-crane-adapter

当前实现的`scow-scheduluer-adapter-interface`版本：v1.8.0

对应CraneSched v1.0.0

## Build

Requires [Buf]([Buf](https://buf.build/docs/installation/)).

```bash
# Generate code from latest scow-slurm-adapter
make protos
make cranesched

# Build
make build

```

**项目部署文档地址**：[deploy 文档](https://github.com/PKUHPC/scow-crane-adapter/blob/master/docs/crane适配器部署文档.md)