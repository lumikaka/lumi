'use strict';

class RuntimeError extends Error {
  constructor(code, message, details = '') {
    super(message);
    this.code = code;
    this.details = details;
  }
}

const failure = (code, message, details = '') => ({
  success: false, data: null, error: { code, message, details },
});
const envelope = (error) => error instanceof RuntimeError
  ? failure(error.code, error.message, error.details)
  : failure('runtime_failed', '本地运行时失败。', '重新执行运行诊断；不要自动重放写操作。');
function aborted(signal) {
  if (signal?.aborted) throw new RuntimeError('request_cancelled', '调用已取消。', '其他任务的连接保持可用。');
}
function validEnvelope(value) {
  return value && typeof value === 'object' && typeof value.success === 'boolean'
    && Object.hasOwn(value, 'data') && (value.success
      ? !value.error
      : value.data === null && typeof value.error?.code === 'string'
        && typeof value.error.message === 'string' && typeof value.error.details === 'string');
}
module.exports = { RuntimeError, failure, envelope, aborted, validEnvelope };
