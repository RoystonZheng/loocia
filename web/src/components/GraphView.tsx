import { WordCloud } from './WordCloud'

// GraphView is the 「图谱」 main view: the interactive keyword cloud, full height.
export function GraphView() {
  return (
    <div className="graph-page">
      <header className="page-head">
        <h1>图谱</h1>
        <p className="page-sub">词云看热度，点一个词看它跟谁连着</p>
      </header>
      <WordCloud />
    </div>
  )
}
