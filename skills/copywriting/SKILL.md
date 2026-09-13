---
name: copywriting
description: "Write conversion-focused content in the user's voice: social posts, ad copy, headlines/hook lines, email campaigns, landing-page sections, product descriptions, with CTA options and A/B variants. Use whenever the user asks to write or improve content meant to sell, attract, or persuade — 'viết bài quảng cáo', 'viết caption', content Facebook/Zalo/TikTok, email marketing, landing page copy, product description, chỉnh sửa câu bán hàng, A/B test copy. Triggers: viết content, viết bài, caption, quảng cáo, email marketing, headline, landing page, CTA, copywriting, sell copy. Do NOT use for technical docs, reports, or non-persuasive writing (answer directly)."
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - brief
  - platform
outputs:
  - copy_draft
allowed-tools:
  - filesystem
quality-gates:
  - brief_confirmed
  - variants_provided
---

# Copywriting (conversion content)

Write copy that sells: hook → value → proof → action, tuned to the platform
and the user's voice.

## Before writing (brief check)

Missing any of these and the copy is a guess — ask (`ask_options` if it fits):
1. **Offer:** what exactly is being sold/promised, price if public.
2. **Audience:** who buys, what pain keeps them up at night, how they talk.
3. **Platform & shape:** Facebook post? Zalo broadcast? TikTok hook? Email?
   Length limits differ wildly.
4. **Voice:** if the user has past writing samples, read them first and mirror
   sentence length, vocabulary, and emoji habits — say "theo giọng anh đang
   dùng" when mirroring. If none, ask formal / thân thiện / hài hước.
5. **Constraint:** banned words, competitor names, legal claims (never invent
   guarantees, "trị dứt điểm", "100%" — flag risky claims when the user
   proposes them).

## Craft rules

- **First line wins:** on social, the first line decides scroll-past. Lead
  with the customer's pain or the surprising outcome — never with the brand
  name.
- **One idea per variant.** Give **3 distinct angles** (e.g. pain-led,
  proof-led, offer-led), not 3 rewrites of the same sentence — label each.
- **CTA matched to temperature:** cold audience → soft ("xem thêm", "tìm hiểu
  thêm"); warm → direct ("inbox để được tư vấn", "đặt hàng ngay"). Offer 2 CTA
  options per draft.
- **Specifics beat adjectives:** "giao trong 2 giờ" > "giao hàng nhanh";
  "1.200+ khách đã dùng" > "được tin tưởng". If the user hasn't provided the
  number, mark it `[cần số liệu]` instead of inventing it.
- **Localize, don't translate:** Vietnamese copy for Vietnamese buyers —
  natural sentence rhythm, correct register (anh/chị/em), no stiff
  calque phrasing.
- **Read it aloud test:** if a sentence needs a second read to parse, shorten
  it before delivering.

## Delivery format

```
Góc tiếp cận 1 — [tên góc] (pain-led)
<copy>
CTA: <option A> / <option B>

Góc tiếp cận 2 — ...
```
Then one line: what to A/B first and why. Deliver in the chat (and save
campaigns to `content/<campaign>.md` when the user iterates on them).

## Revision discipline

When the user says "chưa hay", ask WHICH part fell flat (hook? offer? CTA?) —
rewriting blind produces another guess. Change one variable per revision so
the user can compare.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| User says "viết cho hay hơn" with no brief | Ask the 5 brief questions first — do not draft blind |
| Copy feels generic | Replace every adjective with a specific fact/number/image; cut empty intensifiers (rất, siêu, tuyệt vời) |
| Claims risk being false/illegal | Flag the claim, propose a defensible alternative — never just soften the wording |
| Emoji over/under use | Match the user's own recent posts; when unknown, default minimal |
