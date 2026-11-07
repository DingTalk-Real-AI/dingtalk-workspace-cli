# Token index

`colorToken` and `IconName` use protocol enums. Prefer a registered full ID for `sizeToken`; other strings depend on host compatibility or fallback. Follow each field's Schema for container colors. Use a given Token for one meaning within a card.

## Colors: `ColorToken` (57 items)

- **yellow**: `extended_yellow0_color`, `extended_yellow1_color`, `extended_yellow6_color`
- **orange**: `extended_orange0_color`, `extended_orange1_color`, `extended_orange6_color`, `common_orange1_color`
- **red**: `extended_red0_color`, `extended_red1_color`, `extended_red6_color`, `common_red1_color`
- **magenta**: `extended_magenta0_color`, `extended_magenta1_color`, `extended_magenta6_color`
- **purple**: `extended_purple0_color`, `extended_purple1_color`, `extended_purple6_color`
- **deeppurple**: `extended_deeppurple0_color`, `extended_deeppurple1_color`, `extended_deeppurple6_color`
- **blue**: `extended_blue0_color`, `extended_blue1_color`, `extended_blue6_color`
- **darkgreen**: `extended_darkgreen0_color`, `extended_darkgreen1_color`, `extended_darkgreen6_color`
- **green**: `extended_green0_color`, `extended_green1_color`, `extended_green6_color`, `common_green1_color`
- **level**: `common_level1_base_color`, `common_level2_base_color`, `common_level3_base_color`, `common_level4_base_color`
- **stamp**: `common_stamp_color`
- **link**: `common_link_color`
- **bg**: `common_bg_color`
- **fg**: `common_fg_z1_color`
- **line**: `common_line_light_color`, `common_line_hard_color`
- **white**: `common_white1_color`, `common_white2_color`, `common_white3_color`, `common_white4_color`, `common_white5_color`, `common_white6_color`
- **black**: `common_black1_color`, `common_black2_color`, `common_black3_color`, `common_black4_color`, `common_black5_color`, `common_black6_color`
- **theme**: `theme_primary3_color`, `theme_primary2_color`, `theme_primary_hover_color`, `theme_primary1_color`, `theme_primary_press_color`

| Token | Meaning |
|---|---|
| `extended_yellow0_color` | Yellow background color; used for background filling in areas o… |
| `extended_yellow1_color` | Yellow border color; used for outlining areas of this color sch… |
| `extended_yellow6_color` | Yellow text color; used for text emphasis in this color scheme. |
| `extended_orange0_color` | Orange background color; used for area background filling in th… |
| `extended_orange1_color` | Orange border color; used for outlining areas of this color sch… |
| `extended_orange6_color` | Orange text color; used for text emphasis in this color scheme. |
| `extended_red0_color` | Red background color; used for background filling in areas of t… |
| `extended_red1_color` | Red border color; used for the stroke of areas in this color sc… |
| `extended_red6_color` | Red text color; used for emphasizing text in this color scheme. |
| `extended_magenta0_color` | Magenta background color; used for background filling in areas … |
| `extended_magenta1_color` | Magenta border color; used for outlining areas of this color sc… |
| `extended_magenta6_color` | Magenta text color; used for text emphasis in this color scheme. |
| `extended_purple0_color` | Purple background color; used for background filling in areas o… |
| `extended_purple1_color` | Purple border color; used for the stroke of areas in this color… |
| `extended_purple6_color` | Purple text color; used for emphasizing text in this color sche… |
| `extended_deeppurple0_color` | Deep purple background color; used for background filling in ar… |
| `extended_deeppurple1_color` | Deep purple border color; used for outlining areas of this colo… |
| `extended_deeppurple6_color` | Deep purple text color; used for text emphasis in this color sc… |
| `extended_blue0_color` | Blue background color; used for background filling in areas of … |
| `extended_blue1_color` | Blue border color; used for the stroke of areas in this color s… |
| `extended_blue6_color` | Blue text color; used for emphasizing text in this color scheme. |
| `extended_darkgreen0_color` | Dark green background color; used for area background filling i… |
| `extended_darkgreen1_color` | Dark green border color; used for outlining areas of this color… |
| `extended_darkgreen6_color` | Dark green text color; used for text emphasis in this color sch… |
| `extended_green0_color` | Green background color; used for background filling in areas of… |
| `extended_green1_color` | Green border color; used for outlining areas of this color sche… |
| `extended_green6_color` | Green text color; used for text emphasis in this color scheme. |
| `common_level1_base_color` | Primary text and icons; used for titles, body text, and other m… |
| `common_level2_base_color` | Secondary text and icons; used for secondary information and au… |
| `common_level3_base_color` | Tertiary text and icons; used for weaker supplementary informat… |
| `common_level4_base_color` | Text and icon color in a disabled state; only conveys visual st… |
| `common_stamp_color` | Watermark color; used for low-interference watermark content. |
| `common_link_color` | Hyperlink text and highlight border color. |
| `common_bg_color` | Background color of the area; used for the background of a page… |
| `common_fg_z1_color` | Foreground area background color; used for content areas on top… |
| `common_line_light_color` | Weak divider; used for lighter content separation. |
| `common_line_hard_color` | Strong divider line; used for content separation where a cleare… |
| `common_red1_color` | Danger state color; used to convey dangerous or erroneous seman… |
| `common_orange1_color` | Warning state color; used for prompts that require the user's a… |
| `common_green1_color` | Success state color; used for success or completion feedback. |
| `common_white1_color` | White mask; used for white overlay, actual transparency is dete… |
| `common_white2_color` | White mask (designed as 40% level); used for white overlays, ac… |
| `common_white3_color` | White mask (designed as 30% level); used for white overlays, ac… |
| `common_white4_color` | White mask (designed as 20% level); used for white overlays, ac… |
| `common_white5_color` | White mask (designed as 10% level); used for white overlay, act… |
| `common_white6_color` | White mask (designed as 5% level); used for white overlay, actu… |
| `common_black1_color` | Black mask; used for black overlay, actual transparency is dete… |
| `common_black2_color` | Black mask (designed as 40% level); used for black overlays, ac… |
| `common_black3_color` | Black mask (designed as 30% level); used for black overlays, ac… |
| `common_black4_color` | Black mask (designed as 20% level); used for black overlays, ac… |
| `common_black5_color` | Black mask (designed as 10% level); used for black overlays, ac… |
| `common_black6_color` | Black mask (designed as 5% level); used for black overlays, act… |
| `theme_primary3_color` | Theme background color; used for the background of areas that c… |
| `theme_primary2_color` | Light theme color; used for lighter theme color highlights and … |
| `theme_primary_hover_color` | Theme hover color; used for the visual feedback of the theme co… |
| `theme_primary1_color` | Theme primary color; used for main theme emphasis. |
| `theme_primary_press_color` | Theme pressed color; used for the visual feedback of the theme … |

## Font sizes: `SizeToken` (12 items)

| Token | Meaning |
|---|---|
| `common_largetitle_text_style__font_size` | Main title, used as the heading for a page or major content section. |
| `common_supertitle_text_style__font_size` | Extra-large title, used as the main title for a page or module. |
| `common_h1_text_style__font_size` | Level 1 heading, used for card or content area titles. |
| `common_h2_text_style__font_size` | Level 2 heading, used for content-group headings. |
| `common_h3_text_style__font_size` | Level 3 heading, used for smaller-level content titles. |
| `common_h4_text_style__font_size` | Level 4 heading, used for subheadings or supplementary information unde… |
| `common_body_text_style__font_size` | Main text, used for the primary content. |
| `common_action_text_style__font_size` | Action text, used for buttons, links, and other operational copy. |
| `common_description_text_style__font_size` | Auxiliary explanation, used for secondary information and supplementary… |
| `common_footnote_text_style__font_size` | Footnotes, used for sources, notes, image captions, or status tags. |
| `common_tiny_text_style__font_size` | Small text prompts, used as subtle hints in space-constrained scenarios. |
| `common_subhead_text_style__font_size` | Subtitle, used for supplementary information below the title. |

## Icons: `IconName` (24 items)

| Token | Meaning |
|---|---|
| `Search_L_outlined` | Search. |
| `Language_L_outlined` | Language-switch icon. |
| `CodeProgram_L_outlined` | Code. |
| `Picture_L_outlined` | Image. |
| `Folder_L_outlined` | Folder. |
| `Edit_L_outlined` | Edit or pen icon. |
| `ListView_L_outlined` | List View 1. |
| `ManagementBackground_L_outlined` | Admin console icon. |
| `Tool_L_outlined` | Tools. |
| `Delete_L_outlined` | Delete. |
| `SpinLoading_L_outlined` | Loading spinner icon. |
| `UploadOne_L_outlined` | Upload. |
| `DownloadAndSave_L_outlined` | Download and save. |
| `Check_L_outlined` | Check mark. |
| `Copy_L_outlined` | Copy. |
| `Close_L_outlined` | Close, cross. |
| `More_L_outlined` | More, ellipsis, three dots. |
| `Error_L_outlined` | Warning, error reminder, circle with exclamation mark. |
| `InformationThere_L_outlined` | Information prompt, circular letter i. |
| `Setting_L_outlined` | Settings, gear. |
| `ExpandThere_L_outlined` | Expand. |
| `Collapse_L_outlined` | Collapse. |
| `Link_L_outlined` | Link, chain. |
| `Share_L_outlined` | Share, forward. |


---
Generated from `references/protocol/` (fingerprint `cf4611469d2c3f5e`). Do not edit this index: protocol changes regenerate it. For exact fields and a minimal example, query a name as shown in the Skill entrypoint; the whole catalog need not enter the model context.
