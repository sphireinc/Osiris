import Prism from 'prismjs';

// Kept local to the reference: this grammar describes the public Osiris syntax
// without pulling every bundled Prism language into the documentation site.
export const osirisGrammar = {
  comment: /#.*/,
  string: {
    pattern: /"(?:\\.|[^"\\\r\n])*"/,
    greedy: true,
  },
  keyword: /\b(?:as|assert|await|break|cancel|case|catch|continue|default|defer|else|emit|enum|export|false|for|func|if|import|in|let|let\?|macro|match|none|null|package|parallel|return|spawn|switch|true|try|while)\b/,
  'type-name': /\b(?:bool|datetime|duration|enum|f32|f64|function|i8|i16|i32|i64|list|location|null|string|tensor|u8|u16|u32|u64)\b/,
  'builtin': /\b(?:error|err|ok|some|timeout|trap|panic|assert|read|write|openread|openwrite|readline|readchunk|writechunk|flush|close)\b(?=\s*\()/,
  number: /\b(?:0[xX][\da-fA-F_]+|0[bB][01_]+|0[oO][0-7_]+|\d[\d_]*(?:\.\d[\d_]*)?(?:[eE][+-]?\d+)?)(?:u8|u16|u32|u64|i8|i16|i32|i64|f32|f64|bf16|[ubdfi])?\b/,
  operator: /(?:=>|==|!=|<=|>=|&&|\|\||\?\?|\?\.|\.\.|[+*/%<>=!?:|&-])/,
  punctuation: /[{}\[\]();,.]/,
};

Prism.languages.osiris = osirisGrammar;
