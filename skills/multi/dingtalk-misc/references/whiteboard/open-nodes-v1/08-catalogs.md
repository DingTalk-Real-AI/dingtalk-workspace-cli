# OpenNodes V1 — dml-v1 Geometry 和 Icon 完整目录

> 本文件是 DWS OpenNodes V1 协议的拆分章节。按需读取入口见
> [协议索引](../open-nodes-v1.md)。

## 附录 A：dml-v1 geometry 目录

写入时在以下名称前加 `dml:`，例如 `rect` 写成 `dml:rect`。当前目录共
183 项，目录版本由 `catalogVersion = "dml-v1"` 标识。

```text
accentBorderCallout1 accentBorderCallout2 accentBorderCallout3
accentCallout1 accentCallout2 accentCallout3 actionButtonBackPrevious
actionButtonBeginning actionButtonBlank actionButtonDocument actionButtonEnd
actionButtonForwardNext actionButtonHelp actionButtonHome
actionButtonInformation actionButtonMovie actionButtonReturn actionButtonSound
active allGeneralization arc attribute bentArrow bentUpArrow bevel bind blockArc
borderCallout1 borderCallout2 borderCallout3 bracePair bracketPair callout1
callout2 callout3 can chevron chord circularArrow cloud cloudCallout comment
component control convert corner cube curvedDownArrow curvedLeftArrow
curvedRightArrow curvedUpArrow dataStorage decagon delete diagStripe diamond
dodecagon donut doubleWave downArrow downArrowCallout ellipse ellipseRibbon
ellipseRibbon2 entity entitySet flowChartAlternateProcess flowChartCollate
flowChartConnector flowChartDecision flowChartDelay flowChartDisplay
flowChartDocument flowChartExtract flowChartInputOutput flowChartInternalStorage
flowChartMagneticDisk flowChartMagneticDrum flowChartMagneticTape
flowChartManualInput flowChartManualOperation flowChartMerge
flowChartMultidocument flowChartOffpageConnector flowChartOnlineStorage
flowChartOr flowChartPredefinedProcess flowChartPreparation
flowChartPunchedCard flowChartPunchedTape flowChartSort
flowChartSummingJunction flowChartTerminator foldedCorner frame generalization
halfFrame heart heptagon hexagon history homePlate horizontalDivCircle
horizontalScroll irregularSeal1 irregularSeal2 leftArrow leftArrowCallout
leftBrace leftBracket leftRightArrow leftRightArrowCallout leftRightUpArrow
leftUpArrow lightningBolt mathDivide mathEqual mathMinus mathMultiply
mathNotEqual mathPlus moon multiClass multiValuedAttribute noSmoking node
nonIsoscelesTrapezoid notchedRightArrow octagon parallelogram pentagon person
pie plaque plus quadArrow quadArrowCallout receiveSignal rect relationship
ribbon ribbon2 rightArrow rightArrowCallout rightBrace rightBracket round1Rect
round2DiagRect round2SameRect roundRect rtTriangle smileyFace snip1Rect
snip2DiagRect snip2SameRect snipRoundRect star10 star12 star16 star24 star32
star4 star5 star6 star7 star8 stripedRightArrow sun teardrop triangle upArrow
upArrowCallout upDownArrow user uturnArrow verticalDivCircle verticalScroll
wave weakEntitySet weakRelationship wedgeEllipseCallout wedgeRectCallout
wedgeRoundRectCallout
```

## 附录 B：dml-v1 icon 目录

写入时必须使用完整的 `<group>/<name>`。当前共 4 组 49 项，目录版本由
`catalogVersion = "dml-v1"` 标识。

| group | 数量 | name（组成 `group/name`） |
| --- | ---: | --- |
| `emoji` | 14 | `happy`、`smile`、`laugh`、`fighting`、`like`、`ok`、`please`、`face-plam`、`tears-of-joy`、`cry`、`question`、`face-with-sweat`、`bloody-nose`、`doggy` |
| `tools` | 21 | `pad`、`blue-note`、`yellow-notes`、`chart`、`chart-2`、`pencil`、`pen`、`bag`、`rocket`、`fire`、`gold`、`light`、`pin`、`red-flag`、`tea`、`island`、`ball`、`lucky-fish`、`coffee`、`milky-tea`、`pan` |
| `priority` | 7 | `priority-1`、`priority-2`、`priority-3`、`priority-4`、`priority-5`、`priority-6`、`priority-7` |
| `task` | 7 | `task-start`、`task-oct`、`task-3oct`、`task-half`、`task-5oct`、`task-7oct`、`task-done` |

注意：`emoji/face-plam` 是 V1 保留的历史兼容枚举值，拼写虽然异常但属于协议值；
传入 `emoji/face-palm` 会被拒绝。
