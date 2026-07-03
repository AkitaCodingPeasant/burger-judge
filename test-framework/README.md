# 期末測試架構

- `template/`: 學生開發模版。預期學生撰寫程式碼的地方，包含了必要的標頭檔與程式框架，當學生透過 OJ 下載題目時，預期下載這個目錄底下的檔案。
- `solution/`: 複製 `template` 後，由助教把題目撰寫完成。
- `spec/`: 測資規範。包含 `case[N].h`，定義了各個測試階段的輸入與預期結果。
- `cmake/`: 本題在 OJ 端測試時，需要的輔助腳本。通常包含 `AddJudge.cmake` 輔助腳本，視情況也可以添加其他的進去。
- `CMakeLists.txt`: 根目錄的 CMakeLists 包含了添加 Judge 的腳本。

```cmake
include(AddJudge)
AddJudge("case1")
AddJudge("case2")
AddJudge("case3")
AddJudge("case4")
AddJudge("case5")
```

`AddJudge` 內的參數，與 `spec` 內的 `*.h` 匹配

## 題目定義

每個題目的 `$PROBLEM/template/CMakeLists.txt` 透過 `project()` 定義了該題目的編號，並根據 AddJudge 添加的測試名稱，生成 `${PROJECT_NAME}-${CASE_NAME}`。

```bash
#!/bin/bash
BUILD_DIR=$(openssl rand -hex 8)
cmake -S problems/006 -B /tmp/oj/$BUILD_DIR
# cmake -S problems/006 -B /tmp/oj/$BUILD_DIR -D SOURCE_ROOT=<原始碼的根目錄，未設定預設啟用 $CMAKE_CURRENT_SOURCE_DIR/solution >
cmake --build /tmp/oj/$BUILD_DIR --parallel
ctest --test-dir /tmp/oj/$BUILD_DIR
# ctest --test-dir /tmp/oj/$BUILD_DIR -V 查看詳細資訊
```

## 如何遷移至 Online Judge 系統

1. 假定學生上傳 `BXXX15YYY-001.zip`
2. 解壓縮 `BXXX15YYY-001.zip` 至 `/path/to/source` 資料夾
    - 該過程可以先使用 `cmake -S /path/to/source -B /path/to/builddir` 來確認該資料夾是否為可設定的 CMake 專案
3. `cmake -S /path/to/problem_root -B /path/to/builddir -D SOURCE_ROOT=/path/to/source` 設定專案
4. `cmake --build /path/to/builddir` 編譯專案
5. `ctest --test-dir /path/to/builddir` 運行所有的測試案例
6. 可直接執行 `/path/to/builddir/${PROJECT_NAME}-${CASE_NAME}` 來查看單一案例的執行結果

---

要系統化的開發自動化 Judge System，我們可以定義幾個參數：

|  Variable Name  | Description                  |
| :-------------: | :--------------------------- |
| `PROBLEMS_ROOT` | 包含所有題目的根目錄         |
|  `UPLOAD_DIR`   | 原始碼上傳後，解壓縮的根目錄 |
|   `BUILDDIR`    | 編譯與測試所在的根目錄       |

若學生上傳後的檔案，被解壓縮至 `/tmp/upload/56b60ab1cf95287c`，且打算在 `/tmp/judge/27a85abe71fe1764` 進行編譯：

```bash
#!/bin/bash
PROBLEMS_ROOT="/root/problems"
UPLOAD_DIR="/tmp/upload/$(openssl rand -hex 8)"
BUILDDIR="/tmp/judge/$(openssl rand -hex 8)"

# 若測試的題目為第6題
cmake -S $PROBLEMS_ROOT/113final006 -B $BUILDDIR -D SOURCE_ROOT=$UPLOAD_DIR
cmake --build $BUILDDIR --parallel
ctest --test-dir $BUILDDIR
# command $BUILDDIR/113final006-case1
```

`UPLOAD_DIR` 與 `BUILDDIR` 的階層是可以調整的，例如 `/tmp/upload/學號/題號/隨機ID`、`/tmp/upload/學號-題號-隨機ID` 等

而編譯後的執行檔，例如 `$BUILDDIR/113final006-case1` 是根據 `project` 與 `AddJudge` 定義：

```cmake
project(MyProject1)
...
AddJudge("case1")
AddJudge("12345")
```

就會生成執行檔 `MyProject1-case1` 與 `MyProject1-12345`。與資料夾的名稱無關。

## OJ 系統

若是要上傳題目到 OJ 系統，建議不要有多層資料夾的架構，如

```txt
├── CMakeLists.txt
├── entrypoint.cpp
├── Final006.pdf
├── hint.h
├── include
│ ├── operator.h
│ ├── value.h
│ └── variable.h
├── runtime
│ ├── case.h
│ ├── core.h
│ ├── tap.h
│ └── test.h
└── spec
  ├── case1.h
  └── case2.h
```

應改成

```txt
├── CMakeLists.txt
├── entrypoint.cpp
├── Final006.pdf
├── hint.h
├── operator.h
├── value.h
├── variable.h
├── core.h
├── tap.h
├── test.h
├── case.h
├── case1.h
└── case2.h
```
