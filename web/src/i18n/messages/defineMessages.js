export function defineMessages(definitions) {
  return Object.freeze({
    'zh-Hans': Object.freeze(Object.fromEntries(
      Object.entries(definitions).map(([key, messages]) => [key, messages[0]]),
    )),
    en: Object.freeze(Object.fromEntries(
      Object.entries(definitions).map(([key, messages]) => [key, messages[1]]),
    )),
  })
}
