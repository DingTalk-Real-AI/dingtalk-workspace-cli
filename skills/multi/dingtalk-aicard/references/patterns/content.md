# Content: illustrated information and resources

Use for summaries, recommendations, campaign descriptions, media, and file results. Include images and destinations only when the assets and behavior exist.

| Needed presentation | Optional components | When to choose them |
|---|---|---|
| Prose and citation | `Markdown + Link` | Continuous text, paragraphs, and source. Keep only the prose when no link exists |
| Cover and summary | `Column > Image + Text/Markdown + ButtonGroup` | Content or product with an existing main visual |
| Compact illustrated item | `Row > Image + Column(weight=1, Text, Text)` | Choose an explicit Image variant; `smallFeature` is a thumbnail candidate. Image and text can swap sides |
| Whole item opens details | `Row(action) / Column(action) > illustrated content` | Put a known detail action on the container instead of adding a redundant button; keep unknown behavior static |
| Multiple recommendations | `List > illustrated or text items` | Direct child components for static entries; a template for data-driven entries |
| Multiple evidence images | `Text + ImageList` | Photos or result gallery; consult `ImageCarousel` for individual switching |
| File result | `Text/Markdown + File` | Filename, download resource, and description; do not disguise plain text as an attachment |
| Audio or video | `AudioPlayer` or `Video`, with explanation | Use only with a real playable resource; a static cover is not playback |

Do not omit `Image.variant` in a compact illustrated item: the current default is full-row width and 200 high. Choose dimensions for the asset and layout. `fit=contain` preserves the whole image; `fit=cover` permits cropping. When the outer item opens details and its image is only a thumbnail, `previewEnabled=false` can disable the image's own large preview.

`List.children` accepts component IDs or a template object; `ImageList.images` is an array of image URLs. They have different shapes. A `File` preview is configured by `onPreview`; its appearance alone does not make it clickable.

Keep genuine posters and photos as images, while interface text and controls remain structured. See [image reconstruction](../design.md#image-reconstruction) for brand assets and crop boundaries. For repeated item templates, inspect `Loop` in the [data report example](../protocol/examples/data-report.json).

When content is absent, show a clear empty result instead of an empty media container. If media or attachments arrive later, preserve already-valid summary text and update only the affected content.
