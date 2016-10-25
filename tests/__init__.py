"""Testing helpers and base classes for better isolation."""

from contextlib import contextmanager
import errno
import logging
import os
import StringIO
import subprocess
from tempfile import NamedTemporaryFile
import unittest

from mock import patch

import utility
# For client_past_deadline
from jujupy import (
    EnvJujuClient,
    JujuData,
    )
from datetime import (
    datetime,
    timedelta,
    )


@contextmanager
def stdout_guard():
    stdout = StringIO.StringIO()
    with patch('sys.stdout', stdout):
        yield
    if stdout.getvalue() != '':
        raise AssertionError(
            'Value written to stdout: {}'.format(stdout.getvalue()))


def use_context(test_case, context):
    result = context.__enter__()
    test_case.addCleanup(context.__exit__, None, None, None)
    return result


class TestCase(unittest.TestCase):
    """TestCase provides a better isolated version of unittest.TestCase."""

    log_level = logging.INFO
    test_environ = {}

    def setUp(self):
        super(TestCase, self).setUp()

        def _must_not_Popen(*args, **kwargs):
            """Tests may patch Popen but should never call it."""
            self.fail("subprocess.Popen(*{!r}, **{!r}) called".format(
                args, kwargs))

        self.addCleanup(setattr, subprocess, "Popen", subprocess.Popen)
        subprocess.Popen = _must_not_Popen

        self.addCleanup(setattr, os, "environ", os.environ)
        os.environ = dict(self.test_environ)

        setup_test_logging(self, self.log_level)

    def assertIsTrue(self, expr, msg=None):
        """Assert that expr is the True object."""
        self.assertIs(True, expr, msg)

    def assertIsFalse(self, expr, msg=None):
        """Assert that expr is the False object."""
        self.assertIs(False, expr, msg)


class FakeHomeTestCase(TestCase):
    """FakeHomeTestCase creates an isolated home dir for Juju to use."""

    def setUp(self):
        super(FakeHomeTestCase, self).setUp()
        self.home_dir = use_context(self, utility.temp_dir())
        os.environ["HOME"] = self.home_dir
        os.environ["PATH"] = os.path.join(self.home_dir, ".local", "bin")
        os.mkdir(os.path.join(self.home_dir, ".juju"))


def setup_test_logging(testcase, level=None):
    log = logging.getLogger()
    testcase.addCleanup(setattr, log, 'handlers', log.handlers)
    log.handlers = []
    testcase.log_stream = StringIO.StringIO()
    handler = logging.StreamHandler(testcase.log_stream)
    handler.setFormatter(logging.Formatter("%(levelname)s %(message)s"))
    log.addHandler(handler)
    if level is not None:
        testcase.addCleanup(log.setLevel, log.level)
        log.setLevel(level)


# suppress nosetests
setup_test_logging.__test__ = False


@contextmanager
def parse_error(test_case):
    stderr = StringIO.StringIO()
    with test_case.assertRaises(SystemExit):
        with patch('sys.stderr', stderr):
            yield stderr


@contextmanager
def temp_os_env(key, value):
    org_value = os.environ.get(key, '')
    os.environ[key] = value
    try:
        yield
    finally:
        os.environ[key] = org_value


# Testing tools: ported from jujupy.py.
def assert_juju_call(test_case, mock_method, client, expected_args,
                     call_index=None):
    """Check a mock's positional arguments.

    :param test_case: The test case currently being run.
    :param mock_mothod: The mock object to be checked.
    :param client: Ignored.
    :param expected_args: The expected positional arguments for the call.
    :param call_index: Index of the call to check, if None checks first call
    and checks for only one call."""
    if call_index is None:
        test_case.assertEqual(len(mock_method.mock_calls), 1)
        call_index = 0
    empty, args, kwargs = mock_method.mock_calls[call_index]
    test_case.assertEqual(args, (expected_args,))


class FakePopen(object):
    """Create an artifical version of the Popen class."""

    def __init__(self, out, err, returncode):
        self._out = out
        self._err = err
        self._code = returncode

    def communicate(self):
        self.returncode = self._code
        return self._out, self._err

    def poll(self):
        return self._code


@contextmanager
def observable_temp_file():
    """Get a name which is used to create temporary files in the context."""
    temporary_file = NamedTemporaryFile(delete=False)
    try:
        with temporary_file as temp_file:
            with patch('jujupy.NamedTemporaryFile',
                       return_value=temp_file):
                with patch.object(temp_file, '__exit__'):
                    yield temp_file
    finally:
        try:
            os.unlink(temporary_file.name)
        except OSError as e:
            # File may have already been deleted, e.g. by temp_yaml_file.
            if e.errno != errno.ENOENT:
                raise


# Fake Juju ?
@contextmanager
def client_past_deadline(client=None):
    """Create a client patched to be past its deadline."""
    if client is None:
        client = EnvJujuClient(JujuData('local', juju_home=''), None, None)
    soft_deadline = datetime(2015, 1, 2, 3, 4, 6)
    now = soft_deadline + timedelta(seconds=1)
    client._backend.soft_deadline = soft_deadline
    with patch.object(client._backend, '_now', return_value=now,
                      autospec=True):
        yield client
