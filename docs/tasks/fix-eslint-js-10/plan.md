# fix(deps): 升 @eslint/js 到 v10 并修复 no-useless-assignment

## 背景

Dependabot #103 将 `@eslint/js` 从 `^9` 升到 `^10`。v10 的 `recommended` 收紧，新增 `no-useless-assignment` 规则；CI 的 Frontend lint（3 个平台）因此各报 1 个 error 而失败：

```
src/components/ui/Split.tsx
  237:13  error  The value assigned to 'nextSize' is not used in subsequent statements  no-useless-assignment
```

同一轮另有 40 个 warning（多为 `react-hooks/*`），不阻塞 CI，与本次升级无关。

#103 的分支 `dependabot/npm_and_yarn/eslint/js-10.0.1` 标记 `maintainerCanModify=false`，维护者无法直接向该分支推送修复，因此改为独立分支重做同一升级。

## 改动

- `package.json`：`"@eslint/js": "^9"` → `"^10"`
- `pnpm-lock.yaml`：`pnpm install` 重算
- `src/components/ui/Split.tsx`：去掉第 237 行对 `nextSize` 的无效初始化，由第 239 行直接声明赋值

  ```diff
  -        let nextSize = nextStart - delta;
           prevSize = Math.max(prevMin, Math.min(prevMax, prevSize));
  -        nextSize = prevStart + nextStart - prevSize;
  +        let nextSize = prevStart + nextStart - prevSize;
  ```

  语义不变：原初始值在第 239 行被无条件覆盖、从未被读取；`nextStart` 仍在该行用于计算。

## 验证

- `pnpm install --frozen-lockfile`（锁文件一致性）
- `pnpm exec eslint .` → 0 error
- `pnpm exec prettier --check .`、`pnpm typecheck`、`pnpm test`
- CI：lint / test × 3 平台全绿

## 关联

- Supersedes #103
