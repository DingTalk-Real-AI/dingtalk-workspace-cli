# Component index (47 total; 10 common, 37 others)

Choose a component here, then query its fields and minimal example as shown in the Skill entrypoint. Start with common components and inspect other groups only when needed.
Required fields include `id` and `component`. Child references show the field path to child components; `[]` denotes an array item.

| Component | Group | Required | Child references | Purpose |
|---|---|---|---|---|
| `Button` | Common components | action, child, id | child | Runs a connected action; its label belongs in the child component. |
| `Card` | Common components | child, id | child | Single-child card container with padding, background, and rounded corners. |
| `Column` | Common components | children, id | children | Child nodes are stacked from top to bottom. |
| `Divider` | Common components | id | — | Horizontal or vertical separator line. |
| `Icon` | Common components | id, name | — | DingTalk icon set and loading status. |
| `Image` | Common components | id, url | — | Image display with a dark-theme image alternative. |
| `Markdown` | Common components | content, id | — | Markdown headings, lists, code, and quotes. |
| `Row` | Common components | children, id | children | Child nodes are arranged from left to right. |
| `Tag` | Common components | id, text | — | Compact status or category label styled with theme and variant. |
| `Text` | Common components | id, text | — | Text, font size, color, and streaming state. |
| `ButtonGroup` | Composition | buttons, id | — | Groups inline buttons; static prototypes may omit item actions. |
| `CardHeader` | Composition | id, title | trailing | Unified card header with a theme, dynamic title, dark-theme icon, and trailing content. |
| `CollapsiblePanel` | Composition | children, id, title | children | Unified collapsible panel, collapsed by default. |
| `ColumnLayout` | Composition | children, id | children | Responsive columns from one to six. |
| `GridLayout` | Composition | children, id | children | Grid layout supporting static children and data templates. |
| `Link` | Composition | id, text | — | Clickable link text and navigation. |
| `List` | Composition | children, id | children | Arranges components or data templates horizontally or vertically. |
| `Loop` | Composition | children, id | children | Data-driven repeated-rendering container. |
| `ScrollView` | Composition | id | children | A horizontal or vertical scroll container. |
| `Stack` | Composition | child, id | child, overlays.[].child | Z-axis stacking container: A **base layer** determines the size, on which 0~N **overlay… |
| `Tabs` | Composition | id, tabs | tabs.[].child | Tab header and content panel. |
| `CheckBox` | Inputs | id, label, value | — | Boolean selection. |
| `CheckableImageList` | Inputs | id, images, value | — | Selectable layout for one to nine images, cropped to 4:3 with row and column gaps of 12. |
| `CheckboxListMulti` | Inputs | children, id | children | Multi-select list and local state. |
| `ChoicePicker` | Inputs | id, options, value | — | Selects one or more options; variant determines selection mode. |
| `ConversationPicker` | Inputs | id, value | — | Call the DingTalk conversation selector. |
| `DateTimeInput` | Inputs | id, value | — | Date and time picker entry. |
| `ImageUpload` | Inputs | id, value | — | Call DingTalk image upload, fill the upload result back into the `value` binding path, … |
| `InputList` | Inputs | id, value | — | Input list whose rows can be added and removed. |
| `NumberInput` | Inputs | id, value | — | Numeric input entry that opens an input panel and displays the result. |
| `Rating` | Inputs | id, value | — | Star rating. |
| `Slider` | Inputs | id, max, value | — | Numeric slider. |
| `Switch` | Inputs | id | — | Switch state and size. |
| `TextField` | Inputs | id, label | — | Collects text and saves input through a value binding. |
| `UserPicker` | Inputs | id, value | — | Opens the DingTalk contact picker to select users. |
| `AudioPlayer` | Data and media | id, url | — | Audio cover, title, and playback progress. |
| `Avatar` | Data and media | id | — | Avatar image. |
| `AvatarGroup` | Data and media | id, items | — | Avatar stacking and overflow count. |
| `Chart` | Data and media | data, id | — | Shows trends, comparisons, or proportions using contract-defined data. |
| `Countdown` | Data and media | endTime, id | — | Countdown and end event. |
| `ElapsedTime` | Data and media | id | — | Elapsed-time counter. |
| `File` | Data and media | fileName, id, url | — | File artifact. |
| `ImageCarousel` | Data and media | id, items | — | DingTalk image carousel component. |
| `ImageList` | Data and media | id, images | — | Multiple image thumbnails and quantity aggregation. |
| `ProgressBar` | Data and media | id, value | — | Shows completion as a continuous bar or compact blocks. |
| `Table` | Data and media | data, id | — | Shows structured records for row-by-row review with local pagination. |
| `Video` | Data and media | id, url | — | Cover, inline playback, and control bar. |

---
Generated from `references/protocol/` (fingerprint `7005139e5267c69e`). Do not edit this index: protocol changes regenerate it. For exact fields and a minimal example, query a name as shown in the Skill entrypoint; the whole catalog need not enter the model context.
