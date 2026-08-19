## Bench: esutils IsIdentifierNameES6 (ported) vs node

```
Bench: esutils IsIdentifierNameES6 (300000 iters x 6 inputs)  arg:string
  side                       wall(ms)    mem(MB)        ops/sec
  node isIdentifierNameES6         98.8       48.6       18213193
  go IsIdentifierNameES6         78.0        0.2       23076923
  Go speedup: 1.27x
```
